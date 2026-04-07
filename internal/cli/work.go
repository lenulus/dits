package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store"
	"github.com/lenulus/pf/internal/workops"
	"github.com/spf13/cobra"
)

var workCmd = &cobra.Command{
	Use:   "work",
	Short: "Manage work items",
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

func openWorkOps() (*workops.WorkOps, error) { return workops.Open() }

func workItemDisplayID(wi *domain.WorkItem) string {
	if wi.SharedID != "" {
		return string(wi.SharedID)
	}
	return string(wi.ID)
}

func resolveWI(ctx context.Context, w *workops.WorkOps, ref string) (*domain.WorkItem, error) {
	return w.ResolveWorkItem(ctx, ref)
}

// --- Commands ---

var workCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new work item",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		body, _ := cmd.Flags().GetString("body")
		labels, _ := cmd.Flags().GetStringSlice("label")
		kind, _ := cmd.Flags().GetString("kind")

		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()

		res, err := w.CreateWorkItem(context.Background(), kind, title, body, labels)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s (%s): %s [%s]\n", res.SharedID, res.WorkItem.ID, res.WorkItem.Title, res.WorkItem.Kind)
		return nil
	},
}

var workListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List work items",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()

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

		items, err := w.ListWorkItems(context.Background(), filter, all)
		if err != nil {
			return err
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
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()

		ctx := context.Background()
		wi, err := w.ResolveWorkItem(ctx, args[0])
		if err != nil {
			return err
		}

		if jsonOutput(cmd) {
			return printJSON(wi)
		}

		id := workItemDisplayID(wi)
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

		privateLabels, _ := w.Proj.DB.GetPrivateLabels(ctx, wi.ID)
		annotations, _ := w.Proj.DB.GetAnnotations(ctx, wi.ID)
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
	Use:  "comment <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, _ := cmd.Flags().GetString("body")
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		_, err = w.Comment(ctx, wi.ID, body)
		return err
	},
}

var workCloseCmd = &cobra.Command{
	Use:  "close <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		if wi.Status == "closed" {
			fmt.Println("Already closed.")
			return nil
		}
		if _, err := w.Close(ctx, wi.ID); err != nil {
			return err
		}
		fmt.Printf("Closed %s\n", workItemDisplayID(wi))
		return nil
	},
}

var workReopenCmd = &cobra.Command{
	Use:  "reopen <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		if wi.Status != "closed" {
			fmt.Println("Not closed.")
			return nil
		}
		if _, err := w.Reopen(ctx, wi.ID); err != nil {
			return err
		}
		fmt.Printf("Reopened %s\n", workItemDisplayID(wi))
		return nil
	},
}

var workStatusCmd = &cobra.Command{
	Use:  "status <id> <status>",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		if _, err := w.SetStatus(ctx, wi.ID, wi.Status, args[1]); err != nil {
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
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		if _, err := w.AddLabel(ctx, wi.ID, args[1]); err != nil {
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
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		if _, err := w.RemoveLabel(ctx, wi.ID, args[1]); err != nil {
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
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		if _, err := w.Assign(ctx, wi.ID, domain.ActorID(args[1])); err != nil {
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
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		if _, err := w.Unassign(ctx, wi.ID, domain.ActorID(args[1])); err != nil {
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
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		target, err := resolveWI(ctx, w, args[2])
		if err != nil {
			return fmt.Errorf("target: %w", err)
		}
		if _, err := w.Link(ctx, wi.ID, args[1], target.ID); err != nil {
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
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		target, err := resolveWI(ctx, w, args[2])
		if err != nil {
			return fmt.Errorf("target: %w", err)
		}
		if _, err := w.Unlink(ctx, wi.ID, args[1], target.ID); err != nil {
			return err
		}
		fmt.Printf("Unlinked %s %s %s\n", workItemDisplayID(wi), args[1], workItemDisplayID(target))
		return nil
	},
}

var workAttachCmd = &cobra.Command{
	Use:  "attach <id> <file>",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		res, err := w.Attach(ctx, wi.ID, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Attached %s to %s (%s, %d bytes)\n", res.Filename, workItemDisplayID(wi), res.ArtifactID, res.SizeBytes)
		return nil
	},
}

var workDetachCmd = &cobra.Command{
	Use:  "detach <id> <artifact-id>",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		w, err := openWorkOps()
		if err != nil {
			return err
		}
		defer w.Shutdown()
		ctx := context.Background()
		wi, err := resolveWI(ctx, w, args[0])
		if err != nil {
			return err
		}
		if _, err := w.Detach(ctx, wi.ID, domain.ArtifactID(args[1])); err != nil {
			return err
		}
		fmt.Printf("Detached %s from %s\n", args[1], workItemDisplayID(wi))
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
	workCreateCmd.Flags().StringP("kind", "k", "task", "Work kind")
	workCreateCmd.MarkFlagRequired("title")

	workListCmd.Flags().StringP("status", "s", "", "Filter by status")
	workListCmd.Flags().StringP("kind", "k", "", "Filter by kind")
	workListCmd.Flags().BoolP("all", "a", false, "Include closed")
	workListCmd.Flags().Bool("json", false, "JSON output")

	workShowCmd.Flags().Bool("json", false, "JSON output")

	workCommentCmd.Flags().StringP("body", "b", "", "Comment body (required)")
	workCommentCmd.MarkFlagRequired("body")
}
