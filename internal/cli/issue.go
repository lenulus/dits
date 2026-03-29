package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

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
		if title == "" {
			return fmt.Errorf("--title is required")
		}

		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		issueID := domain.NewCanonicalID()
		eventID := domain.NewEventID()
		now := time.Now().UTC()

		event := domain.Event{
			ID:             eventID,
			IssueID:        issueID,
			Type:           domain.EventIssueCreated,
			ParentEventIDs: nil,
			ActorID:        proj.Config.ActorID,
			Timestamp:      now,
			Payload:        domain.MustMarshalPayload(domain.IssueCreatedPayload{Title: title, Body: body}),
		}

		if err := proj.DB.AppendEvents(ctx, []domain.Event{event}); err != nil {
			return fmt.Errorf("appending event: %w", err)
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
			fmt.Println("No issues found.")
			return nil
		}

		for _, iss := range issues {
			id := string(iss.SharedID)
			if id == "" {
				id = string(iss.ID)
			}
			if !all && iss.Status == "closed" {
				continue
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
		if issue.Body != "" {
			fmt.Printf("\n%s\n", issue.Body)
		}

		if len(issue.Comments) > 0 {
			fmt.Printf("\n--- Comments (%d) ---\n", len(issue.Comments))
			for _, c := range issue.Comments {
				fmt.Printf("\n[%s] %s:\n%s\n", c.Timestamp.Format(time.RFC3339), c.ActorID, c.Body)
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

func init() {
	issueCmd.AddCommand(issueCreateCmd)
	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueShowCmd)
	issueCmd.AddCommand(issueCommentCmd)
	issueCmd.AddCommand(issueCloseCmd)
	issueCmd.AddCommand(issueReopenCmd)

	issueCreateCmd.Flags().StringP("title", "t", "", "Issue title")
	issueCreateCmd.Flags().StringP("body", "b", "", "Issue body")
	issueCreateCmd.MarkFlagRequired("title")

	issueListCmd.Flags().StringP("status", "s", "", "Filter by status")
	issueListCmd.Flags().BoolP("all", "a", false, "Show all issues including closed")

	issueCommentCmd.Flags().StringP("body", "b", "", "Comment body")
	issueCommentCmd.MarkFlagRequired("body")
}

// --- Helpers ---

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

func appendAndMaterialize(ctx context.Context, proj *project.Project, issueID domain.CanonicalID, event domain.Event) error {
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
