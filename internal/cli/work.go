package cli

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"encoding/json"

	"github.com/lenulus/pf/internal/blob"
	"github.com/lenulus/pf/internal/crypto"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/store"
	"github.com/spf13/cobra"
)

var workCmd = &cobra.Command{
	Use:   "work",
	Short: "Manage work items",
}

var workCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new work item",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		body, _ := cmd.Flags().GetString("body")
		labels, _ := cmd.Flags().GetStringSlice("label")
		kind, _ := cmd.Flags().GetString("kind")
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

		sharedID, err := proj.DB.AllocateSharedID(ctx, workItemID, proj.Config.ProjectKey)
		if err != nil {
			return fmt.Errorf("allocating shared ID: %w", err)
		}

		fmt.Printf("Created %s (%s): %s [%s]\n", sharedID, workItemID, title, kind)
		return nil
	},
}

var workListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List work items",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		status, _ := cmd.Flags().GetString("status")
		kind, _ := cmd.Flags().GetString("kind")
		all, _ := cmd.Flags().GetBool("all")

		filter := store.WorkItemFilter{}
		if status != "" {
			filter.Status = status
		}
		if kind != "" {
			filter.Kind = kind
		}

		ctx := context.Background()
		items, err := proj.DB.ListWorkItems(ctx, filter)
		if err != nil {
			return err
		}

		if !all {
			var filtered []domain.WorkItem
			for _, wi := range items {
				if wi.Status != "closed" {
					filtered = append(filtered, wi)
				}
			}
			items = filtered
		}

		if len(items) == 0 {
			if !jsonOutput(cmd) {
				fmt.Println("No work items found.")
			} else {
				fmt.Println("[]")
			}
			return nil
		}

		if jsonOutput(cmd) {
			return printJSON(items)
		}

		for _, wi := range items {
			id := string(wi.SharedID)
			if id == "" {
				id = string(wi.ID)
			}
			fmt.Printf("%-12s %-12s %-14s %s\n", id, wi.Kind, wi.Status, wi.Title)
		}
		return nil
	},
}

var workShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show work item details",
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

		fmt.Printf("Work Item: %s\n", id)
		fmt.Printf("ID:        %s\n", wi.ID)
		fmt.Printf("Kind:      %s\n", wi.Kind)
		fmt.Printf("Title:     %s\n", wi.Title)
		fmt.Printf("Status:    %s\n", wi.Status)
		fmt.Printf("Priority:  %s\n", wi.Priority)
		fmt.Printf("Created:   %s by %s\n", wi.CreatedAt.Format(time.RFC3339), wi.CreatedBy)
		fmt.Printf("Updated:   %s\n", wi.UpdatedAt.Format(time.RFC3339))

		if wi.Blocked {
			fmt.Printf("Blocked:   %s\n", wi.BlockedReason)
		}
		if wi.LeaseHolder != nil {
			fmt.Printf("Leased by: %s (expires %s)\n", *wi.LeaseHolder, wi.LeaseExpiresAt.Format(time.RFC3339))
		}
		if wi.CurrentAttempt != nil {
			fmt.Printf("Attempt:   %s\n", *wi.CurrentAttempt)
		}
		if wi.RetainedOutcomeRef != nil {
			fmt.Printf("Retained:  %s\n", *wi.RetainedOutcomeRef)
		}

		if len(wi.Labels) > 0 {
			fmt.Printf("Labels:    %s\n", strings.Join(wi.Labels, ", "))
		}
		if len(wi.Assignees) > 0 {
			assignees := make([]string, len(wi.Assignees))
			for i, a := range wi.Assignees {
				assignees[i] = string(a)
			}
			fmt.Printf("Assigned:  %s\n", strings.Join(assignees, ", "))
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
				t := a.ArtifactType
				if t == "" {
					t = "-"
				}
				fmt.Printf("  %s  %-8s  %s  (%d bytes)  %s\n", a.ID, t, a.Filename, a.SizeBytes, a.ContentHash)
			}
		}

		if len(wi.Attempts) > 0 {
			fmt.Printf("\n--- Attempts (%d) ---\n", len(wi.Attempts))
			for _, a := range wi.Attempts {
				completed := ""
				if a.CompletedAt != nil {
					completed = fmt.Sprintf(" -> %s", a.CompletedAt.Format(time.RFC3339))
				}
				fmt.Printf("  #%d %s  %-10s  %s%s\n", a.Number, a.AttemptID, a.Status, a.StartedAt.Format(time.RFC3339), completed)
			}
		}

		if len(wi.Checkpoints) > 0 {
			fmt.Printf("\n--- Checkpoints (%d) ---\n", len(wi.Checkpoints))
			for _, cp := range wi.Checkpoints {
				fmt.Printf("  [%.0f%%] %s (%s)\n", cp.Progress*100, cp.Summary, cp.Timestamp.Format(time.RFC3339))
			}
		}

		if len(wi.Observations) > 0 {
			fmt.Printf("\n--- Observations (%d) ---\n", len(wi.Observations))
			for _, obs := range wi.Observations {
				fmt.Printf("  [%s] %s\n", obs.Timestamp.Format(time.RFC3339), obs.Summary)
			}
		}

		if len(wi.Findings) > 0 {
			fmt.Printf("\n--- Findings (%d) ---\n", len(wi.Findings))
			for _, f := range wi.Findings {
				retracted := ""
				if f.Retracted {
					retracted = " [RETRACTED]"
				}
				fmt.Printf("  [%.0f%%] %s%s\n", f.Confidence*100, f.Statement, retracted)
			}
		}

		if len(wi.Outcomes) > 0 || wi.RetainedOutcomeRef != nil {
			fmt.Printf("\n--- Outcomes ---\n")
			if wi.RetainedOutcomeRef != nil {
				fmt.Printf("  Retained: %s\n", *wi.RetainedOutcomeRef)
			}
			for _, oc := range wi.Outcomes {
				evalInfo := ""
				if oc.EvalRef != "" {
					evalInfo = fmt.Sprintf(" (eval: %s)", oc.EvalRef)
				}
				fmt.Printf("  [%s] %s %s %s%s\n", oc.Decision, oc.SubjectKind, oc.SubjectRef, oc.Reason, evalInfo)
			}
		}

		if len(wi.Comments) > 0 {
			fmt.Printf("\n--- Comments (%d) ---\n", len(wi.Comments))
			for _, c := range wi.Comments {
				fmt.Printf("\n[%s] %s:\n%s\n", c.Timestamp.Format(time.RFC3339), c.ActorID, c.Body)
			}
		}

		// Overlay data.
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

var workCommentCmd = &cobra.Command{
	Use:   "comment <id>",
	Short: "Add a comment",
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

var workCloseCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Close a work item",
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
			fmt.Println("Already closed.")
			return nil
		}

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkClosed,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ClosedPayload{}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Closed %s\n", workItemDisplayID(wi))
		return nil
	},
}

var workReopenCmd = &cobra.Command{
	Use:   "reopen <id>",
	Short: "Reopen a work item",
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
			fmt.Println("Not closed.")
			return nil
		}

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkReopened,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(struct{}{}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Reopened %s\n", workItemDisplayID(wi))
		return nil
	},
}

var workStatusCmd = &cobra.Command{
	Use:   "status <id> <status>",
	Short: "Change work item status",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
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

		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkStatusSet,
			ParentEventIDs: heads, MetaVersion: meta.Version,
			ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.StatusSetPayload{From: wi.Status, To: args[1]}),
		}

		if err := domain.ValidateEvent(event, meta); err != nil {
			return fmt.Errorf("validation: %w", err)
		}
		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Status: %s -> %s on %s\n", wi.Status, args[1], workItemDisplayID(wi))
		return nil
	},
}

var workLabelAddCmd = &cobra.Command{
	Use:  "label-add <id> <slug>",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
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
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkLabelAdded,
			ParentEventIDs: heads, MetaVersion: meta.Version,
			ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.LabelPayload{LabelSlug: args[1]}),
		}
		if err := domain.ValidateEvent(event, meta); err != nil {
			return fmt.Errorf("validation: %w", err)
		}
		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Added label %q to %s\n", args[1], workItemDisplayID(wi))
		return nil
	},
}

var workLabelRemoveCmd = &cobra.Command{
	Use:  "label-remove <id> <slug>",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()
		ctx := context.Background()
		meta, _ := loadMeta(ctx, proj)
		wi, err := resolveWorkItem(ctx, proj, args[0])
		if err != nil {
			return err
		}
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkLabelRemoved,
			ParentEventIDs: heads, MetaVersion: meta.Version,
			ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.LabelPayload{LabelSlug: args[1]}),
		}
		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Removed label %q from %s\n", args[1], workItemDisplayID(wi))
		return nil
	},
}

var workAssignCmd = &cobra.Command{
	Use:  "assign <id> <actor>",
	Args: cobra.ExactArgs(2),
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
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkAssigned,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.AssignPayload{Assignee: domain.ActorID(args[1])}),
		}
		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Assigned %s to %s\n", workItemDisplayID(wi), args[1])
		return nil
	},
}

var workUnassignCmd = &cobra.Command{
	Use:  "unassign <id> <actor>",
	Args: cobra.ExactArgs(2),
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
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkUnassigned,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.AssignPayload{Assignee: domain.ActorID(args[1])}),
		}
		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Unassigned %s from %s\n", args[1], workItemDisplayID(wi))
		return nil
	},
}

var workLinkCmd = &cobra.Command{
	Use:  "link <id> <type> <target-id>",
	Args: cobra.ExactArgs(3),
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
		target, err := resolveWorkItem(ctx, proj, args[2])
		if err != nil {
			return fmt.Errorf("target: %w", err)
		}
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkLinked,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.RelationPayload{RelationType: args[1], TargetWorkItem: target.ID}),
		}
		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("%s %s %s\n", workItemDisplayID(wi), args[1], workItemDisplayID(target))
		return nil
	},
}

var workUnlinkCmd = &cobra.Command{
	Use:  "unlink <id> <type> <target-id>",
	Args: cobra.ExactArgs(3),
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
		target, err := resolveWorkItem(ctx, proj, args[2])
		if err != nil {
			return fmt.Errorf("target: %w", err)
		}
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkUnlinked,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.RelationPayload{RelationType: args[1], TargetWorkItem: target.ID}),
		}
		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Unlinked %s %s %s\n", workItemDisplayID(wi), args[1], workItemDisplayID(target))
		return nil
	},
}

var workAttachCmd = &cobra.Command{
	Use:   "attach <id> <file>",
	Short: "Attach a file as an artifact",
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

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		artID := domain.NewArtifactID()
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkArtifactAdded,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ArtifactAddedPayload{
				ArtifactID: artID, ContentHash: hash, Filename: filename, MimeType: mimeType, SizeBytes: size,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Attached %s to %s (%s, %d bytes)\n", filename, workItemDisplayID(wi), artID, size)
		return nil
	},
}

var workDetachCmd = &cobra.Command{
	Use:  "detach <id> <artifact-id>",
	Args: cobra.ExactArgs(2),
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
			return fmt.Errorf("artifact %s not found", artID)
		}
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkArtifactRemoved,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ArtifactRemovedPayload{ArtifactID: artID}),
		}
		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Detached %s from %s\n", artID, workItemDisplayID(wi))
		return nil
	},
}

func init() {
	workCmd.AddCommand(workCreateCmd)
	workCmd.AddCommand(workListCmd)
	workCmd.AddCommand(workShowCmd)
	workCmd.AddCommand(workCommentCmd)
	workCmd.AddCommand(workCloseCmd)
	workCmd.AddCommand(workReopenCmd)
	workCmd.AddCommand(workStatusCmd)
	workCmd.AddCommand(workLabelAddCmd)
	workCmd.AddCommand(workLabelRemoveCmd)
	workCmd.AddCommand(workAssignCmd)
	workCmd.AddCommand(workUnassignCmd)
	workCmd.AddCommand(workLinkCmd)
	workCmd.AddCommand(workUnlinkCmd)
	workCmd.AddCommand(workAttachCmd)
	workCmd.AddCommand(workDetachCmd)

	workCreateCmd.Flags().StringP("title", "t", "", "Title (required)")
	workCreateCmd.Flags().StringP("body", "b", "", "Body/description")
	workCreateCmd.Flags().StringSliceP("label", "l", nil, "Labels")
	workCreateCmd.Flags().StringP("kind", "k", "task", "Work kind (task, issue, investigation, execution, plan, decision, handoff, eval)")
	workCreateCmd.MarkFlagRequired("title")

	workListCmd.Flags().StringP("status", "s", "", "Filter by status")
	workListCmd.Flags().StringP("kind", "k", "", "Filter by kind")
	workListCmd.Flags().BoolP("all", "a", false, "Include closed")
	workListCmd.Flags().Bool("json", false, "JSON output")

	workShowCmd.Flags().Bool("json", false, "JSON output")

	workCommentCmd.Flags().StringP("body", "b", "", "Comment body (required)")
	workCommentCmd.MarkFlagRequired("body")
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
	// Protocol validation: check coordination invariants against current state.
	existing, _ := proj.DB.GetWorkItem(ctx, workItemID)
	if err := domain.ValidateProtocol(event, existing); err != nil {
		return err
	}

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

	existing, _ = proj.DB.GetWorkItem(ctx, workItemID)
	if existing != nil && existing.SharedID != "" {
		wi.SharedID = existing.SharedID
	}

	return proj.DB.UpsertWorkItem(ctx, wi)
}

func workItemDisplayID(wi *domain.WorkItem) string {
	if wi.SharedID != "" {
		return string(wi.SharedID)
	}
	return string(wi.ID)
}
