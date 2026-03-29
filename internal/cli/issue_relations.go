package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/spf13/cobra"
)

var issueLinkCmd = &cobra.Command{
	Use:   "link <issue-id> <relation-type> <target-issue-id>",
	Short: "Link an issue to another (e.g., blocks, relates_to, duplicates)",
	Args:  cobra.ExactArgs(3),
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
		target, err := resolveIssue(ctx, proj, args[2])
		if err != nil {
			return fmt.Errorf("target issue: %w", err)
		}

		heads, err := proj.DB.GetHeads(ctx, issue.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issue.ID,
			Type:           domain.EventIssueLinked,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.RelationPayload{
				RelationType: args[1],
				TargetIssue:  target.ID,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		fmt.Printf("%s %s %s\n", issueDisplayID(issue), args[1], issueDisplayID(target))
		return nil
	},
}

var issueUnlinkCmd = &cobra.Command{
	Use:   "unlink <issue-id> <relation-type> <target-issue-id>",
	Short: "Remove a link between issues",
	Args:  cobra.ExactArgs(3),
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
		target, err := resolveIssue(ctx, proj, args[2])
		if err != nil {
			return fmt.Errorf("target issue: %w", err)
		}

		heads, err := proj.DB.GetHeads(ctx, issue.ID)
		if err != nil {
			return err
		}

		event := domain.Event{
			ID:             domain.NewEventID(),
			IssueID:        issue.ID,
			Type:           domain.EventIssueUnlinked,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.RelationPayload{
				RelationType: args[1],
				TargetIssue:  target.ID,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		fmt.Printf("Unlinked %s %s %s\n", issueDisplayID(issue), args[1], issueDisplayID(target))
		return nil
	},
}

var issueAssignCmd = &cobra.Command{
	Use:   "assign <issue-id> <actor-id>",
	Short: "Assign an issue to an actor",
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
			Type:           domain.EventIssueAssigned,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.AssignPayload{Assignee: domain.ActorID(args[1])}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		fmt.Printf("Assigned %s to %s\n", issueDisplayID(issue), args[1])
		return nil
	},
}

var issueUnassignCmd = &cobra.Command{
	Use:   "unassign <issue-id> <actor-id>",
	Short: "Unassign an actor from an issue",
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
			Type:           domain.EventIssueUnassigned,
			ParentEventIDs: heads,
			MetaVersion:    meta.Version,
			ActorID:        proj.Config.ActorID,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.AssignPayload{Assignee: domain.ActorID(args[1])}),
		}

		if err := appendAndMaterialize(ctx, proj, issue.ID, event); err != nil {
			return err
		}

		fmt.Printf("Unassigned %s from %s\n", args[1], issueDisplayID(issue))
		return nil
	},
}

func init() {
	issueCmd.AddCommand(issueLinkCmd)
	issueCmd.AddCommand(issueUnlinkCmd)
	issueCmd.AddCommand(issueAssignCmd)
	issueCmd.AddCommand(issueUnassignCmd)
}
