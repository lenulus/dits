package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// --- Annotations (key-value notes on issues, local only) ---

var issueAnnotateCmd = &cobra.Command{
	Use:   "annotate <issue-id> <key> <value>",
	Short: "Set a local annotation on an issue (not synced)",
	Args:  cobra.ExactArgs(3),
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

		if err := proj.DB.SetAnnotation(ctx, issue.ID, args[1], args[2]); err != nil {
			return err
		}
		fmt.Printf("Set annotation %s=%s on %s (local only)\n", args[1], args[2], issueDisplayID(issue))
		return nil
	},
}

var issueAnnotationsCmd = &cobra.Command{
	Use:   "annotations <issue-id>",
	Short: "Show local annotations for an issue",
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

		annotations, err := proj.DB.GetAnnotations(ctx, issue.ID)
		if err != nil {
			return err
		}
		if len(annotations) == 0 {
			fmt.Println("No annotations.")
			return nil
		}

		keys := make([]string, 0, len(annotations))
		for k := range annotations {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %-20s %s\n", k, annotations[k])
		}
		return nil
	},
}

var issueAnnotateDeleteCmd = &cobra.Command{
	Use:   "annotate-delete <issue-id> <key>",
	Short: "Delete a local annotation",
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

		if err := proj.DB.DeleteAnnotation(ctx, issue.ID, args[1]); err != nil {
			return err
		}
		fmt.Printf("Deleted annotation %q on %s\n", args[1], issueDisplayID(issue))
		return nil
	},
}

// --- Private labels (local only, not synced) ---

var issuePrivateLabelAddCmd = &cobra.Command{
	Use:   "private-label-add <issue-id> <label>",
	Short: "Add a private label to an issue (not synced)",
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

		if err := proj.DB.AddPrivateLabel(ctx, issue.ID, args[1]); err != nil {
			return err
		}
		fmt.Printf("Added private label %q to %s (local only)\n", args[1], issueDisplayID(issue))
		return nil
	},
}

var issuePrivateLabelRemoveCmd = &cobra.Command{
	Use:   "private-label-remove <issue-id> <label>",
	Short: "Remove a private label from an issue",
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

		if err := proj.DB.RemovePrivateLabel(ctx, issue.ID, args[1]); err != nil {
			return err
		}
		fmt.Printf("Removed private label %q from %s\n", args[1], issueDisplayID(issue))
		return nil
	},
}

func init() {
	issueCmd.AddCommand(issueAnnotateCmd)
	issueCmd.AddCommand(issueAnnotationsCmd)
	issueCmd.AddCommand(issueAnnotateDeleteCmd)
	issueCmd.AddCommand(issuePrivateLabelAddCmd)
	issueCmd.AddCommand(issuePrivateLabelRemoveCmd)
}
