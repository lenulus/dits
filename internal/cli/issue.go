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
		issueType, _ := cmd.Flags().GetString("type")
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

		if issueType == "" {
			issueType = "task"
		}

		issueID := domain.NewCanonicalID()
		now := time.Now().UTC()

		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issueID,
			Type:           domain.EventIssueCreated,
			ParentEventIDs: nil,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      now,
			Payload:        domain.MustMarshalPayload(domain.IssueCreatedPayload{Title: title, Body: body, TypeSlug: issueType}),
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
			heads, _ := proj.DB.GetHeads(ctx, issueID)
			labelEvt := domain.Event{
				ID:             domain.NewEventID(),
				IssueID:        issueID,
				Type:           domain.EventIssueLabelAdded,
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
		events, err := proj.DB.GetEventsForIssue(ctx, issueID)
		if err != nil {
			return err
		}
		ordered := domain.CausalOrder(events)
		issue, err := domain.Reduce(ordered)
		if err != nil {
			return err
		}

		if err := proj.DB.UpsertIssue(ctx, issue); err != nil {
			return err
		}

		// Allocate shared ID.
		sharedID, err := proj.DB.AllocateSharedID(ctx, issueID, proj.Config.ProjectKey)
		if err != nil {
			return fmt.Errorf("allocating shared ID: %w", err)
		}

		fmt.Printf("Created %s (%s): %s\n", sharedID, issueID, title)
		return nil
	},
}

var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		status, _ := cmd.Flags().GetString("status")
		all, _ := cmd.Flags().GetBool("all")

		filter := store.IssueFilter{}
		if !all && status == "" {
			// Default: show non-closed
			filter.Status = "" // we'll handle this below
		} else if status != "" {
			filter.Status = status
		}

		ctx := context.Background()
		issues, err := proj.DB.ListIssues(ctx, filter)
		if err != nil {
			return err
		}

		if len(issues) == 0 {
			if !jsonOutput(cmd) {
				fmt.Println("No issues found.")
			} else {
				fmt.Println("[]")
			}
			return nil
		}

		// Filter closed if not --all.
		if !all {
			var filtered []domain.Issue
			for _, iss := range issues {
				if iss.Status != "closed" {
					filtered = append(filtered, iss)
				}
			}
			issues = filtered
		}

		if jsonOutput(cmd) {
			return printJSON(issues)
		}

		for _, iss := range issues {
			id := string(iss.SharedID)
			if id == "" {
				id = string(iss.ID)
			}
			fmt.Printf("%-12s %-12s %s\n", id, iss.Status, iss.Title)
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
		issue, err := resolveIssue(ctx, proj, args[0])
		if err != nil {
			return err
		}

		if jsonOutput(cmd) {
			return printJSON(issue)
		}

		id := string(issue.SharedID)
		if id == "" {
			id = string(issue.ID)
		}

		fmt.Printf("Issue:    %s\n", id)
		fmt.Printf("ID:       %s\n", issue.ID)
		fmt.Printf("Title:    %s\n", issue.Title)
		fmt.Printf("Status:   %s\n", issue.Status)
		fmt.Printf("Type:     %s\n", issue.TypeSlug)
		fmt.Printf("Priority: %s\n", issue.Priority)
		fmt.Printf("Created:  %s by %s\n", issue.CreatedAt.Format(time.RFC3339), issue.CreatedBy)
		fmt.Printf("Updated:  %s\n", issue.UpdatedAt.Format(time.RFC3339))

		if len(issue.Labels) > 0 {
			fmt.Printf("Labels:   %s\n", strings.Join(issue.Labels, ", "))
		}
		if len(issue.Assignees) > 0 {
			assignees := make([]string, len(issue.Assignees))
			for i, a := range issue.Assignees {
				assignees[i] = string(a)
			}
			fmt.Printf("Assigned: %s\n", strings.Join(assignees, ", "))
		}
		if len(issue.Relations) > 0 {
			fmt.Println("\nRelations:")
			for _, r := range issue.Relations {
				fmt.Printf("  %s %s\n", r.Type, r.TargetIssue)
			}
		}
		if issue.Body != "" {
			fmt.Printf("\n%s\n", issue.Body)
		}

		if len(issue.Attachments) > 0 {
			fmt.Printf("\n--- Attachments (%d) ---\n", len(issue.Attachments))
			for _, a := range issue.Attachments {
				fmt.Printf("  %s  %s  (%d bytes)  %s\n", a.ID, a.Filename, a.SizeBytes, a.ContentHash)
			}
		}

		if len(issue.Comments) > 0 {
			fmt.Printf("\n--- Comments (%d) ---\n", len(issue.Comments))
			for _, c := range issue.Comments {
				fmt.Printf("\n[%s] %s:\n%s\n", c.Timestamp.Format(time.RFC3339), c.ActorID, c.Body)
			}
		}

		// Overlay data (local only).
		privateLabels, _ := proj.DB.GetPrivateLabels(ctx, issue.ID)
		annotations, _ := proj.DB.GetAnnotations(ctx, issue.ID)
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
		issue, err := resolveIssue(ctx, proj, args[0])
		if err != nil {
			return err
		}

		heads, err := proj.DB.GetHeads(ctx, issue.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issue.ID,
			Type:           domain.EventIssueCommented,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.CommentPayload{Body: body}),
		}

		return appendAndMaterialize(ctx, proj, issue.ID, event)
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
		issue, err := resolveIssue(ctx, proj, args[0])
		if err != nil {
			return err
		}
		if issue.Status == "closed" {
			fmt.Println("Issue is already closed.")
			return nil
		}

		heads, err := proj.DB.GetHeads(ctx, issue.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issue.ID,
			Type:           domain.EventIssueClosed,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(struct{}{}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		id := string(issue.SharedID)
		if id == "" {
			id = string(issue.ID)
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
		issue, err := resolveIssue(ctx, proj, args[0])
		if err != nil {
			return err
		}
		if issue.Status != "closed" {
			fmt.Println("Issue is not closed.")
			return nil
		}

		heads, err := proj.DB.GetHeads(ctx, issue.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issue.ID,
			Type:           domain.EventIssueReopened,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(struct{}{}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		id := string(issue.SharedID)
		if id == "" {
			id = string(issue.ID)
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
		issue, err := resolveIssue(ctx, proj, args[0])
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

		// Compute hash.
		hash, size, err := blob.ComputeFileHash(filePath)
		if err != nil {
			return err
		}

		// Store blob locally.
		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer f.Close()

		if err := proj.Blobs.Put(ctx, hash, f); err != nil {
			return fmt.Errorf("storing blob: %w", err)
		}

		// Determine mime type.
		filename := filepath.Base(filePath)
		mimeType := mime.TypeByExtension(filepath.Ext(filename))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		// Create event.
		heads, err := proj.DB.GetHeads(ctx, issue.ID)
		if err != nil {
			return err
		}

		attID := domain.NewAttachmentID()
		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issue.ID,
			Type:           domain.EventAttachmentAdded,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.AttachmentAddedPayload{
				AttachmentID: attID,
				ContentHash:  hash,
				Filename:     filename,
				MimeType:     mimeType,
				SizeBytes:    size,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		id := string(issue.SharedID)
		if id == "" {
			id = string(issue.ID)
		}
		fmt.Printf("Attached %s to %s (%s, %d bytes)\n", filename, id, attID, size)
		return nil
	},
}

var issueAttachmentsCmd = &cobra.Command{
	Use:     "attachments <issue-id>",
	Short:   "List attachments for an issue",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		issue, err := resolveIssue(ctx, proj, args[0])
		if err != nil {
			return err
		}

		if len(issue.Attachments) == 0 {
			fmt.Println("No attachments.")
			return nil
		}

		for _, a := range issue.Attachments {
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
		issue, err := resolveIssue(ctx, proj, args[0])
		if err != nil {
			return err
		}

		attID := domain.AttachmentID(args[1])
		found := false
		for _, a := range issue.Attachments {
			if a.ID == attID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("attachment %s not found on this issue", attID)
		}

		heads, err := proj.DB.GetHeads(ctx, issue.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issue.ID,
			Type:           domain.EventAttachmentRemoved,
			ParentEventIDs: heads,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.AttachmentRemovedPayload{AttachmentID: attID}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		id := string(issue.SharedID)
		if id == "" {
			id = string(issue.ID)
		}
		fmt.Printf("Detached %s from %s\n", attID, id)
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
	issueCreateCmd.Flags().String("type", "task", "Issue type slug")
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

func resolveIssue(ctx context.Context, proj *project.Project, ref string) (*domain.Issue, error) {
	// Try as shared ID first.
	issue, err := proj.DB.GetIssueBySharedID(ctx, domain.SharedID(ref))
	if err != nil {
		return nil, err
	}
	if issue != nil {
		return issue, nil
	}

	// Try as canonical ID.
	issue, err = proj.DB.GetIssue(ctx, domain.CanonicalID(ref))
	if err != nil {
		return nil, err
	}
	if issue != nil {
		return issue, nil
	}

	return nil, fmt.Errorf("issue not found: %s", ref)
}

func signEvent(proj *project.Project, e *domain.Event) error {
	if k := proj.PrivKey(); k != nil {
		return crypto.SignEvent(e, k)
	}
	return nil
}

func appendAndMaterialize(ctx context.Context, proj *project.Project, issueID domain.CanonicalID, event domain.Event) error {
	if err := signEvent(proj, &event); err != nil {
		return fmt.Errorf("signing event: %w", err)
	}

	if err := proj.DB.AppendEvents(ctx, []domain.Event{event}); err != nil {
		return fmt.Errorf("appending event: %w", err)
	}

	events, err := proj.DB.GetEventsForIssue(ctx, issueID)
	if err != nil {
		return err
	}
	ordered := domain.CausalOrder(events)
	issue, err := domain.Reduce(ordered)
	if err != nil {
		return err
	}

	// Preserve shared ID from existing issue.
	existing, _ := proj.DB.GetIssue(ctx, issueID)
	if existing != nil && existing.SharedID != "" {
		issue.SharedID = existing.SharedID
	}

	return proj.DB.UpsertIssue(ctx, issue)
}
