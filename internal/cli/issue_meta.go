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
			Type:           domain.EventIssueLabelAdded,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.LabelPayload{LabelSlug: args[1]}),
		}

		if err := domain.ValidateEvent(event, meta); err != nil {
			return fmt.Errorf("validation: %w", err)
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		fmt.Printf("Added label %q to %s\n", args[1], issueDisplayID(issue))
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
		issue, err := resolveIssue(ctx, proj, args[0])
		if err != nil {
			return err
		}

		heads, err := proj.DB.GetHeads(ctx, issue.ID)
		if err != nil {
			return err
		}

		meta, err := loadMeta(ctx, proj)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issue.ID,
			Type:           domain.EventIssueLabelRemoved,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.LabelPayload{LabelSlug: args[1]}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		fmt.Printf("Removed label %q from %s\n", args[1], issueDisplayID(issue))
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
			Type:           domain.EventIssueStatusSet,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.StatusSetPayload{
				From: issue.Status,
				To:   args[1],
			}),
		}

		if err := domain.ValidateEvent(event, meta); err != nil {
			return fmt.Errorf("validation: %w", err)
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		fmt.Printf("Status changed: %s -> %s on %s\n", issue.Status, args[1], issueDisplayID(issue))
		return nil
	},
}

func issueDisplayID(issue *domain.Issue) string {
	if issue.SharedID != "" {
		return string(issue.SharedID)
	}
	return string(issue.ID)
}

func init() {
	issueCmd.AddCommand(issueLabelAddCmd)
	issueCmd.AddCommand(issueLabelRemoveCmd)
	issueCmd.AddCommand(issueStatusCmd)
}
