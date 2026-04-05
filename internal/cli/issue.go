package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/blob"
	"github.com/lenulus/pf/internal/crypto"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/store"
	"github.com/spf13/cobra"
)

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage issues",
}

var issueCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		body, _ := cmd.Flags().GetString("body")
		labels, _ := cmd.Flags().GetStringSlice("label")
		kind, _ := cmd.Flags().GetString("type")
		if title == "" {
			return fmt.Errorf("--title is required")
		}

		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		meta, err := loadMeta(ctx, proj)
		if err != nil {
			return err
		}

		if kind == "" {
			kind = "task"
		}

		workItemID := domain.NewWorkItemID()
		now := time.Now().UTC()

		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     workItemID,
			Type:           domain.EventWorkCreated,
			ParentEventIDs: nil,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      now,
			Payload:        domain.MustMarshalPayload(domain.WorkCreatedPayload{Title: title, Body: body, Kind: kind}),
		}

		if err := domain.ValidateEvent(event, meta); err != nil {
			return fmt.Errorf("validation: %w", err)
		}
		if err := signEvent(proj, &event); err != nil {
			return fmt.Errorf("signing: %w", err)
		}

		if err := proj.DB.AppendEvents(ctx, []domain.Event{event}); err != nil {
			return fmt.Errorf("appending event: %w", err)
		}

		// Add label events if specified.
		for _, l := range labels {
			heads, _ := proj.DB.GetHeads(ctx, workItemID)
			labelEvt := domain.Event{
				ID:             domain.NewEventID(),
				WorkItemID:     workItemID,
				Type:           domain.EventWorkLabelAdded,
				ParentEventIDs: heads,
				MetaVersion:    meta.Version,
				ActorID:        proj.Config.ActorID,
				Timestamp:      time.Now().UTC(),
				Payload:        domain.MustMarshalPayload(domain.LabelPayload{LabelSlug: l}),
			}
			if err := domain.ValidateEvent(labelEvt, meta); err != nil {
				return fmt.Errorf("validation: %w", err)
			}
			if err := signEvent(proj, &labelEvt); err != nil {
				return fmt.Errorf("signing: %w", err)
			}
			if err := proj.DB.AppendEvents(ctx, []domain.Event{labelEvt}); err != nil {
				return err
			}
		}

		// Materialize.
		events, err := proj.DB.GetEventsForWorkItem(ctx, workItemID)
		if err != nil {
			return err
		}
		ordered := domain.CausalOrder(events)
		wi, err := domain.Reduce(ordered)
		if err != nil {
			return err
		}

		if err := proj.DB.UpsertWorkItem(ctx, wi); err != nil {
			return err
		}

		// Allocate shared ID.
		sharedID, err := proj.DB.AllocateSharedID(ctx, workItemID, proj.Config.ProjectKey)
		if err != nil {
			return fmt.Errorf("allocating shared ID: %w", err)
		}

		fmt.Printf("Created %s (%s): %s\n", sharedID, workItemID, title)
		return nil
	},
}

var issueListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List issues",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		status, _ := cmd.Flags().GetString("status")
		all, _ := cmd.Flags().GetBool("all")

		filter := store.WorkItemFilter{}
		if status != "" {
			filter.Status = status
		}

		ctx := context.Background()
		items, err := proj.DB.ListWorkItems(ctx, filter)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			if !jsonOutput(cmd) {
				fmt.Println("No issues found.")
			} else {
				fmt.Println("[]")
			}
			return nil
		}

		// Filter closed if not --all.
		if !all {
			var filtered []domain.WorkItem
			for _, wi := range items {
				if wi.Status != "closed" {
					filtered = append(filtered, wi)
				}
			}
			items = filtered
		}

		if jsonOutput(cmd) {
			return printJSON(items)
		}

		for _, wi := range items {
			id := string(wi.SharedID)
			if id == "" {
				id = string(wi.ID)
			}
			fmt.Printf("%-12s %-12s %s\n", id, wi.Status, wi.Title)
		}
		return nil
	},
}

var issueShowCmd = &cobra.Command{
	Use:   "show <issue-id>",
	Short: "Show issue details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}

		if jsonOutput(cmd) {
			return printJSON(wi)
		}

		id := string(wi.SharedID)
		if id == "" {
			id = string(wi.ID)
		}

		fmt.Printf("Issue:    %s\n", id)
		fmt.Printf("ID:       %s\n", wi.ID)
		fmt.Printf("Title:    %s\n", wi.Title)
		fmt.Printf("Kind:     %s\n", wi.Kind)
		fmt.Printf("Status:   %s\n", wi.Status)
		fmt.Printf("Priority: %s\n", wi.Priority)
		fmt.Printf("Created:  %s by %s\n", wi.CreatedAt.Format(time.RFC3339), wi.CreatedBy)
		fmt.Printf("Updated:  %s\n", wi.UpdatedAt.Format(time.RFC3339))

		if wi.Blocked {
			fmt.Printf("Blocked:  %s\n", wi.BlockedReason)
		}
		if wi.LeaseHolder != nil {
			fmt.Printf("Leased:   %s\n", *wi.LeaseHolder)
		}

		if len(wi.Labels) > 0 {
			fmt.Printf("Labels:   %s\n", strings.Join(wi.Labels, ", "))
		}
		if len(wi.Assignees) > 0 {
			assignees := make([]string, len(wi.Assignees))
			for i, a := range wi.Assignees {
				assignees[i] = string(a)
			}
			fmt.Printf("Assigned: %s\n", strings.Join(assignees, ", "))
		}
		if len(wi.Relations) > 0 {
			fmt.Println("\nRelations:")
			for _, r := range wi.Relations {
				fmt.Printf("  %s %s\n", r.Type, r.TargetWorkItem)
			}
		}
		if wi.Body != "" {
			fmt.Printf("\n%s\n", wi.Body)
		}

		if len(wi.Artifacts) > 0 {
			fmt.Printf("\n--- Artifacts (%d) ---\n", len(wi.Artifacts))
			for _, a := range wi.Artifacts {
				fmt.Printf("  %s  %s  (%d bytes)  %s\n", a.ID, a.Filename, a.SizeBytes, a.ContentHash)
			}
		}

		if len(wi.Attempts) > 0 {
			fmt.Printf("\n--- Attempts (%d) ---\n", len(wi.Attempts))
			for _, a := range wi.Attempts {
				fmt.Printf("  #%d %s  %s  %s\n", a.Number, a.AttemptID, a.Status, a.StartedAt.Format(time.RFC3339))
			}
		}

		if len(wi.Comments) > 0 {
			fmt.Printf("\n--- Comments (%d) ---\n", len(wi.Comments))
			for _, c := range wi.Comments {
				fmt.Printf("\n[%s] %s:\n%s\n", c.Timestamp.Format(time.RFC3339), c.ActorID, c.Body)
			}
		}

		// Overlay data (local only).
		privateLabels, _ := proj.DB.GetPrivateLabels(ctx, wi.ID)
		annotations, _ := proj.DB.GetAnnotations(ctx, wi.ID)
		if len(privateLabels) > 0 || len(annotations) > 0 {
			fmt.Printf("\n--- Local (not synced) ---\n")
			if len(privateLabels) > 0 {
				fmt.Printf("Private labels: %s\n", strings.Join(privateLabels, ", "))
			}
			if len(annotations) > 0 {
				fmt.Println("Annotations:")
				for k, v := range annotations {
					fmt.Printf("  %-20s %s\n", k, v)
				}
			}
		}

		return nil
	},
}

var issueCommentCmd = &cobra.Command{
	Use:   "comment <issue-id>",
	Short: "Add a comment to an issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, _ := cmd.Flags().GetString("body")
		if body == "" {
			return fmt.Errorf("--body is required")
		}

		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}

		heads, err := proj.DB.GetHeads(ctx, wi.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wi.ID,
			Type:           domain.EventWorkCommented,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.CommentPayload{Body: body}),
		}

		return appendAndMaterialize(ctx, proj, wi.ID, event)
	},
}

var issueCloseCmd = &cobra.Command{
	Use:   "close <issue-id>",
	Short: "Close an issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}
		if wi.Status == "closed" {
			fmt.Println("Issue is already closed.")
			return nil
		}

		heads, err := proj.DB.GetHeads(ctx, wi.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wi.ID,
			Type:           domain.EventWorkClosed,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.ClosedPayload{}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}

		id := string(wi.SharedID)
		if id == "" {
			id = string(wi.ID)
		}
		fmt.Printf("Closed %s\n", id)
		return nil
	},
}

var issueReopenCmd = &cobra.Command{
	Use:   "reopen <issue-id>",
	Short: "Reopen a closed issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}
		if wi.Status != "closed" {
			fmt.Println("Issue is not closed.")
			return nil
		}

		heads, err := proj.DB.GetHeads(ctx, wi.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wi.ID,
			Type:           domain.EventWorkReopened,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(struct{}{}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}

		id := string(wi.SharedID)
		if id == "" {
			id = string(wi.ID)
		}
		fmt.Printf("Reopened %s\n", id)
		return nil
	},
}

var issueAttachCmd = &cobra.Command{
	Use:   "attach <issue-id> <file-path>",
	Short: "Attach a file to an issue",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}

		filePath := args[1]
		info, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
		if info.Size() > blob.MaxBlobSize {
			return fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), blob.MaxBlobSize)
		}

		hash, size, err := blob.ComputeFileHash(filePath)
		if err != nil {
			return err
		}

		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer f.Close()

		if err := proj.Blobs.Put(ctx, hash, f); err != nil {
			return fmt.Errorf("storing blob: %w", err)
		}

		filename := filepath.Base(filePath)
		mimeType := mime.TypeByExtension(filepath.Ext(filename))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		heads, err := proj.DB.GetHeads(ctx, wi.ID)
		if err != nil {
			return err
		}

		artID := domain.NewArtifactID()
		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wi.ID,
			Type:           domain.EventWorkArtifactAdded,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ArtifactAddedPayload{
				ArtifactID:  artID,
				ContentHash: hash,
				Filename:    filename,
				MimeType:    mimeType,
				SizeBytes:   size,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}

		id := string(wi.SharedID)
		if id == "" {
			id = string(wi.ID)
		}
		fmt.Printf("Attached %s to %s (%s, %d bytes)\n", filename, id, artID, size)
		return nil
	},
}

var issueAttachmentsCmd = &cobra.Command{
	Use:   "attachments <issue-id>",
	Short: "List attachments for an issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}

		if len(wi.Artifacts) == 0 {
			fmt.Println("No attachments.")
			return nil
		}

		for _, a := range wi.Artifacts {
			fmt.Printf("%-28s %-30s %8d  %s  %s\n", a.ID, a.Filename, a.SizeBytes, a.MimeType, a.ContentHash)
		}
		return nil
	},
}

var issueDetachCmd = &cobra.Command{
	Use:   "detach <issue-id> <attachment-id>",
	Short: "Remove an attachment from an issue",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}

		artID := domain.ArtifactID(args[1])
		found := false
		for _, a := range wi.Artifacts {
			if a.ID == artID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("artifact %s not found on this work item", artID)
		}

		heads, err := proj.DB.GetHeads(ctx, wi.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wi.ID,
			Type:           domain.EventWorkArtifactRemoved,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.ArtifactRemovedPayload{ArtifactID: artID}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}

		id := string(wi.SharedID)
		if id == "" {
			id = string(wi.ID)
		}
		fmt.Printf("Detached %s from %s\n", artID, id)
		return nil
	},
}

func init() {
	issueCmd.AddCommand(issueCreateCmd)
	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueShowCmd)
	issueCmd.AddCommand(issueCommentCmd)
	issueCmd.AddCommand(issueCloseCmd)
	issueCmd.AddCommand(issueReopenCmd)
	issueCmd.AddCommand(issueAttachCmd)
	issueCmd.AddCommand(issueAttachmentsCmd)
	issueCmd.AddCommand(issueDetachCmd)

	issueCreateCmd.Flags().StringP("title", "t", "", "Issue title")
	issueCreateCmd.Flags().StringP("body", "b", "", "Issue body")
	issueCreateCmd.Flags().StringSliceP("label", "l", nil, "Labels to add (comma-separated or repeated)")
	issueCreateCmd.Flags().String("type", "task", "Work kind slug")
	issueCreateCmd.MarkFlagRequired("title")

	issueListCmd.Flags().StringP("status", "s", "", "Filter by status")
	issueListCmd.Flags().BoolP("all", "a", false, "Show all issues including closed")
	issueListCmd.Flags().Bool("json", false, "Output as JSON")

	issueShowCmd.Flags().Bool("json", false, "Output as JSON")

	issueCommentCmd.Flags().StringP("body", "b", "", "Comment body")
	issueCommentCmd.MarkFlagRequired("body")
}

// --- Helpers ---

func jsonOutput(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func loadMeta(ctx context.Context, proj *project.Project) (*domain.MetaConfig, error) {
	meta, err := proj.DB.GetCurrentMeta(ctx)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, fmt.Errorf("no meta configuration found")
	}
	return meta, nil
}

func loadProject() (*project.Project, error) {
	root, err := project.FindRoot()
	if err != nil {
		return nil, err
	}
	return project.Load(root)
}

func resolveWorkItem(ctx context.Context, proj *project.Project, ref string) (*domain.WorkItem, error) {
	wi, err := proj.DB.GetWorkItemBySharedID(ctx, domain.SharedID(ref))
	if err != nil {
		return nil, err
	}
	if wi != nil {
		return wi, nil
	}

	wi, err = proj.DB.GetWorkItem(ctx, domain.WorkItemID(ref))
	if err != nil {
		return nil, err
	}
	if wi != nil {
		return wi, nil
	}

	return nil, fmt.Errorf("work item not found: %s", ref)
}

func signEvent(proj *project.Project, e *domain.Event) error {
	if k := proj.PrivKey(); k != nil {
		return crypto.SignEvent(e, k)
	}
	return nil
}

func appendAndMaterialize(ctx context.Context, proj *project.Project, workItemID domain.WorkItemID, event domain.Event) error {
	if err := signEvent(proj, &event); err != nil {
		return fmt.Errorf("signing event: %w", err)
	}

	if err := proj.DB.AppendEvents(ctx, []domain.Event{event}); err != nil {
		return fmt.Errorf("appending event: %w", err)
	}

	events, err := proj.DB.GetEventsForWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}
	ordered := domain.CausalOrder(events)
	wi, err := domain.Reduce(ordered)
	if err != nil {
		return err
	}

	existing, _ := proj.DB.GetWorkItem(ctx, workItemID)
	if existing != nil && existing.SharedID != "" {
		wi.SharedID = existing.SharedID
	}

	return proj.DB.UpsertWorkItem(ctx, wi)
}
