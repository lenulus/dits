package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/spf13/cobra"
)

var issueLabelAddCmd = &cobra.Command{
	Use:   "label-add <issue-id> <label-slug>",
	Short: "Add a label to an issue",
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

		heads, err := proj.DB.GetHeads(ctx, wi.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wi.ID,
			Type:           domain.EventWorkLabelAdded,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.LabelPayload{LabelSlug: args[1]}),
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

var issueLabelRemoveCmd = &cobra.Command{
	Use:   "label-remove <issue-id> <label-slug>",
	Short: "Remove a label from an issue",
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

		heads, err := proj.DB.GetHeads(ctx, wi.ID)
		if err != nil {
			return err
		}

		meta, err := loadMeta(ctx, proj)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wi.ID,
			Type:           domain.EventWorkLabelRemoved,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.LabelPayload{LabelSlug: args[1]}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}

		fmt.Printf("Removed label %q from %s\n", args[1], workItemDisplayID(wi))
		return nil
	},
}

var issueStatusCmd = &cobra.Command{
	Use:   "status <issue-id> <status>",
	Short: "Change issue status",
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

		heads, err := proj.DB.GetHeads(ctx, wi.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wi.ID,
			Type:           domain.EventWorkStatusSet,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.StatusSetPayload{
				From: wi.Status,
				To:   args[1],
			}),
		}

		if err := domain.ValidateEvent(event, meta); err != nil {
			return fmt.Errorf("validation: %w", err)
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}

		fmt.Printf("Status changed: %s -> %s on %s\n", wi.Status, args[1], workItemDisplayID(wi))
		return nil
	},
}

func workItemDisplayID(wi *domain.WorkItem) string {
	if wi.SharedID != "" {
		return string(wi.SharedID)
	}
	return string(wi.ID)
}

func init() {
	issueCmd.AddCommand(issueLabelAddCmd)
	issueCmd.AddCommand(issueLabelRemoveCmd)
	issueCmd.AddCommand(issueStatusCmd)
}
