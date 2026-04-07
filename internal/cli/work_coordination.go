package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/spf13/cobra"
)

var workLeaseCmd = &cobra.Command{
	Use:  "lease <id>",
	Args: cobra.ExactArgs(1),
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
		res, err := w.Lease(ctx, wi.ID)
		if err != nil {
			return err
		}
		fmt.Printf("Leased %s (lease %s, expires %s)\n", workItemDisplayID(wi), res.LeaseID, res.LeaseExpiresAt.Format(time.RFC3339))
		return nil
	},
}

var workLeaseReleaseCmd = &cobra.Command{
	Use:  "lease-release <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reason, _ := cmd.Flags().GetString("reason")
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
		if _, err := w.LeaseRelease(ctx, wi.ID, reason); err != nil {
			return err
		}
		fmt.Printf("Released lease on %s\n", workItemDisplayID(wi))
		return nil
	},
}

var workStartCmd = &cobra.Command{
	Use:  "start <id>",
	Args: cobra.ExactArgs(1),
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
		res, err := w.Start(ctx, wi.ID)
		if err != nil {
			return err
		}
		fmt.Printf("Started attempt %s on %s\n", res.AttemptID, workItemDisplayID(wi))
		return nil
	},
}

var workCompleteCmd = &cobra.Command{
	Use:  "complete <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, _ := cmd.Flags().GetString("summary")
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
		attemptID := ""
		if wi.CurrentAttempt != nil {
			attemptID = string(*wi.CurrentAttempt)
		}
		if _, err := w.Complete(ctx, wi.ID, summary); err != nil {
			return err
		}
		fmt.Printf("Completed attempt %s on %s\n", attemptID, workItemDisplayID(wi))
		return nil
	},
}

var workFailCmd = &cobra.Command{
	Use:  "fail <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		errMsg, _ := cmd.Flags().GetString("error")
		retryable, _ := cmd.Flags().GetBool("retryable")
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
		attemptID := ""
		if wi.CurrentAttempt != nil {
			attemptID = string(*wi.CurrentAttempt)
		}
		if _, err := w.Fail(ctx, wi.ID, errMsg, retryable); err != nil {
			return err
		}
		fmt.Printf("Failed attempt %s on %s\n", attemptID, workItemDisplayID(wi))
		return nil
	},
}

var workCheckpointCmd = &cobra.Command{
	Use:  "checkpoint <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, _ := cmd.Flags().GetString("summary")
		progress, _ := cmd.Flags().GetFloat64("progress")
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
		if _, err := w.Checkpoint(ctx, wi.ID, summary, progress); err != nil {
			return err
		}
		fmt.Printf("Checkpoint [%.0f%%] %s on %s\n", progress*100, summary, workItemDisplayID(wi))
		return nil
	},
}

var workBlockCmd = &cobra.Command{
	Use:  "block <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reason, _ := cmd.Flags().GetString("reason")
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
		if _, err := w.Block(ctx, wi.ID, reason); err != nil {
			return err
		}
		fmt.Printf("Blocked %s: %s\n", workItemDisplayID(wi), reason)
		return nil
	},
}

var workUnblockCmd = &cobra.Command{
	Use:  "unblock <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reason, _ := cmd.Flags().GetString("reason")
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
		if _, err := w.Unblock(ctx, wi.ID, reason); err != nil {
			return err
		}
		fmt.Printf("Unblocked %s\n", workItemDisplayID(wi))
		return nil
	},
}

var workObserveCmd = &cobra.Command{
	Use:  "observe <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, _ := cmd.Flags().GetString("summary")
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
		if _, err := w.Observe(ctx, wi.ID, summary); err != nil {
			return err
		}
		fmt.Printf("Observed: %s\n", summary)
		return nil
	},
}

var workFindingCmd = &cobra.Command{
	Use:  "finding <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		statement, _ := cmd.Flags().GetString("statement")
		confidence, _ := cmd.Flags().GetFloat64("confidence")
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
		if _, err := w.Finding(ctx, wi.ID, statement, confidence); err != nil {
			return err
		}
		fmt.Printf("Finding [%.0f%%]: %s\n", confidence*100, statement)
		return nil
	},
}

var workPlanCmd = &cobra.Command{
	Use:  "plan <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, _ := cmd.Flags().GetString("summary")
		plan, _ := cmd.Flags().GetString("plan")
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
		if _, err := w.Plan(ctx, wi.ID, summary, plan); err != nil {
			return err
		}
		fmt.Printf("Plan proposed: %s\n", summary)
		return nil
	},
}

var workHandoffCmd = &cobra.Command{
	Use:  "handoff <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		to, _ := cmd.Flags().GetString("to")
		ctxText, _ := cmd.Flags().GetString("context")
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
		res, err := w.Handoff(ctx, wi.ID, domain.ActorID(to), ctxText)
		if err != nil {
			return err
		}
		fmt.Printf("Handed off %s to %s (%s)\n", workItemDisplayID(wi), to, res.HandoffID)
		return nil
	},
}

var workReviewCmd = &cobra.Command{
	Use:  "review <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scope, _ := cmd.Flags().GetString("scope")
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
		res, err := w.Review(ctx, wi.ID, scope)
		if err != nil {
			return err
		}
		fmt.Printf("Review requested on %s (%s): %s\n", workItemDisplayID(wi), res.ReviewID, scope)
		return nil
	},
}

var workEvalRequestCmd = &cobra.Command{
	Use:  "eval-request <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scope, _ := cmd.Flags().GetString("scope")
		subjectRef, _ := cmd.Flags().GetString("subject")
		subjectKind, _ := cmd.Flags().GetString("subject-kind")
		rubricRef, _ := cmd.Flags().GetString("rubric-ref")
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
		res, err := w.EvalRequest(ctx, wi.ID, scope, subjectKind, subjectRef, rubricRef)
		if err != nil {
			return err
		}
		fmt.Printf("Eval requested on %s (%s): %s\n", workItemDisplayID(wi), res.EvalID, scope)
		return nil
	},
}

var workEvalCompleteCmd = &cobra.Command{
	Use:  "eval-complete <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		evalID, _ := cmd.Flags().GetString("eval-id")
		subjectRef, _ := cmd.Flags().GetString("subject")
		subjectKind, _ := cmd.Flags().GetString("subject-kind")
		rubricRef, _ := cmd.Flags().GetString("rubric-ref")
		verdict, _ := cmd.Flags().GetString("verdict")
		summary, _ := cmd.Flags().GetString("summary")
		metricsRaw, _ := cmd.Flags().GetString("metrics")
		metricsFile, _ := cmd.Flags().GetString("metrics-file")

		var metricsJSON json.RawMessage
		if metricsFile != "" {
			data, err := os.ReadFile(metricsFile)
			if err != nil {
				return fmt.Errorf("reading metrics file: %w", err)
			}
			if !json.Valid(data) {
				return fmt.Errorf("metrics file is not valid JSON")
			}
			metricsJSON = data
		} else if metricsRaw != "" {
			if !json.Valid([]byte(metricsRaw)) {
				return fmt.Errorf("--metrics value is not valid JSON")
			}
			metricsJSON = json.RawMessage(metricsRaw)
		}

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
		if _, err := w.EvalComplete(ctx, wi.ID, evalID, subjectKind, subjectRef, rubricRef, verdict, summary, metricsJSON); err != nil {
			return err
		}
		fmt.Printf("Eval completed on %s: %s\n", workItemDisplayID(wi), verdict)
		return nil
	},
}

var workRetainCmd = &cobra.Command{
	Use:  "retain <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		subjectRef, _ := cmd.Flags().GetString("subject")
		subjectKind, _ := cmd.Flags().GetString("subject-kind")
		reason, _ := cmd.Flags().GetString("reason")
		evalRef, _ := cmd.Flags().GetString("eval-ref")
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
		if _, err := w.Retain(ctx, wi.ID, subjectKind, subjectRef, reason, evalRef); err != nil {
			return err
		}
		fmt.Printf("Retained %s %s on %s\n", subjectKind, subjectRef, workItemDisplayID(wi))
		return nil
	},
}

var workDiscardCmd = &cobra.Command{
	Use:  "discard <id>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		subjectRef, _ := cmd.Flags().GetString("subject")
		subjectKind, _ := cmd.Flags().GetString("subject-kind")
		reason, _ := cmd.Flags().GetString("reason")
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
		if _, err := w.Discard(ctx, wi.ID, subjectKind, subjectRef, reason); err != nil {
			return err
		}
		fmt.Printf("Discarded %s %s on %s\n", subjectKind, subjectRef, workItemDisplayID(wi))
		return nil
	},
}

func init() {
	workCmd.AddCommand(workLeaseCmd)
	workCmd.AddCommand(workLeaseReleaseCmd)
	workCmd.AddCommand(workStartCmd)
	workCmd.AddCommand(workCompleteCmd)
	workCmd.AddCommand(workFailCmd)
	workCmd.AddCommand(workCheckpointCmd)
	workCmd.AddCommand(workBlockCmd)
	workCmd.AddCommand(workUnblockCmd)
	workCmd.AddCommand(workObserveCmd)
	workCmd.AddCommand(workFindingCmd)
	workCmd.AddCommand(workPlanCmd)
	workCmd.AddCommand(workHandoffCmd)
	workCmd.AddCommand(workReviewCmd)
	workCmd.AddCommand(workEvalRequestCmd)
	workCmd.AddCommand(workEvalCompleteCmd)
	workCmd.AddCommand(workRetainCmd)
	workCmd.AddCommand(workDiscardCmd)

	workLeaseReleaseCmd.Flags().String("reason", "", "Release reason")

	workCompleteCmd.Flags().String("summary", "", "Completion summary")

	workFailCmd.Flags().String("error", "", "Error message (required)")
	workFailCmd.Flags().Bool("retryable", false, "Whether the failure is retryable")
	workFailCmd.MarkFlagRequired("error")

	workCheckpointCmd.Flags().StringP("summary", "s", "", "Checkpoint summary (required)")
	workCheckpointCmd.Flags().Float64P("progress", "p", 0, "Progress (0.0 - 1.0)")
	workCheckpointCmd.MarkFlagRequired("summary")

	workBlockCmd.Flags().StringP("reason", "r", "", "Block reason (required)")
	workBlockCmd.MarkFlagRequired("reason")

	workUnblockCmd.Flags().String("reason", "", "Unblock reason")

	workObserveCmd.Flags().StringP("summary", "s", "", "Observation summary (required)")
	workObserveCmd.MarkFlagRequired("summary")

	workFindingCmd.Flags().StringP("statement", "s", "", "Finding statement (required)")
	workFindingCmd.Flags().Float64P("confidence", "c", 0.5, "Confidence (0.0 - 1.0)")
	workFindingCmd.MarkFlagRequired("statement")

	workPlanCmd.Flags().StringP("summary", "s", "", "Plan summary (required)")
	workPlanCmd.Flags().String("plan", "", "Plan text (required)")
	workPlanCmd.MarkFlagRequired("summary")
	workPlanCmd.MarkFlagRequired("plan")

	workHandoffCmd.Flags().String("to", "", "Target actor (required)")
	workHandoffCmd.Flags().String("context", "", "Handoff context (required)")
	workHandoffCmd.MarkFlagRequired("to")
	workHandoffCmd.MarkFlagRequired("context")

	workReviewCmd.Flags().StringP("scope", "s", "", "Review scope (required)")
	workReviewCmd.MarkFlagRequired("scope")

	workEvalRequestCmd.Flags().StringP("scope", "s", "", "Eval scope (required)")
	workEvalRequestCmd.Flags().String("subject", "", "Subject reference")
	workEvalRequestCmd.Flags().String("subject-kind", "", "Subject kind")
	workEvalRequestCmd.Flags().String("rubric-ref", "", "Rubric reference")
	workEvalRequestCmd.MarkFlagRequired("scope")

	workEvalCompleteCmd.Flags().String("eval-id", "", "Eval ID (required)")
	workEvalCompleteCmd.Flags().String("subject", "", "Subject reference")
	workEvalCompleteCmd.Flags().String("subject-kind", "", "Subject kind")
	workEvalCompleteCmd.Flags().String("rubric-ref", "", "Rubric reference")
	workEvalCompleteCmd.Flags().String("verdict", "", "Verdict (required)")
	workEvalCompleteCmd.Flags().String("summary", "", "Summary")
	workEvalCompleteCmd.Flags().String("metrics", "", "Metrics inline JSON")
	workEvalCompleteCmd.Flags().String("metrics-file", "", "Metrics JSON file")
	workEvalCompleteCmd.MarkFlagRequired("eval-id")
	workEvalCompleteCmd.MarkFlagRequired("verdict")

	workRetainCmd.Flags().String("subject", "", "Subject reference (required)")
	workRetainCmd.Flags().String("subject-kind", "", "Subject kind (required)")
	workRetainCmd.Flags().String("reason", "", "Retain reason")
	workRetainCmd.Flags().String("eval-ref", "", "Eval reference")
	workRetainCmd.MarkFlagRequired("subject")
	workRetainCmd.MarkFlagRequired("subject-kind")

	workDiscardCmd.Flags().String("subject", "", "Subject reference (required)")
	workDiscardCmd.Flags().String("subject-kind", "", "Subject kind (required)")
	workDiscardCmd.Flags().String("reason", "", "Discard reason")
	workDiscardCmd.MarkFlagRequired("subject")
	workDiscardCmd.MarkFlagRequired("subject-kind")
}
