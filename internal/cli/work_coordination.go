package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/spf13/cobra"
)

var workLeaseCmd = &cobra.Command{
	Use:   "lease <id>",
	Short: "Lease a work item",
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

		if wi.LeaseHolder != nil {
			return fmt.Errorf("work item is already leased by %s", *wi.LeaseHolder)
		}

		// Determine lease duration from meta policy or default.
		durationSec := 300
		meta, _ := loadMeta(ctx, proj)
		if meta != nil {
			if p := meta.GetLeasePolicy(wi.Kind); p != nil {
				durationSec = p.DefaultDurationSec
			}
		}

		now := time.Now().UTC()
		expiresAt := now.Add(time.Duration(durationSec) * time.Second)
		leaseID := domain.NewLeaseID()

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkLeased,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: now,
			Payload: domain.MustMarshalPayload(domain.LeasedPayload{
				LeaseID: leaseID, LeaseDurationSec: durationSec,
				LeaseExpiresAt: expiresAt, Generation: 1,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Leased %s (lease %s, expires %s)\n", workItemDisplayID(wi), leaseID, expiresAt.Format(time.RFC3339))
		return nil
	},
}

var workLeaseReleaseCmd = &cobra.Command{
	Use:   "lease-release <id>",
	Short: "Release a lease on a work item",
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

		if wi.LeaseHolder == nil {
			fmt.Println("No active lease.")
			return nil
		}

		reason, _ := cmd.Flags().GetString("reason")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkLeaseReleased,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.LeaseReleasedPayload{LeaseID: "lea_released", Reason: reason}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Released lease on %s\n", workItemDisplayID(wi))
		return nil
	},
}

var workStartCmd = &cobra.Command{
	Use:   "start <id>",
	Short: "Start an execution attempt",
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

		attemptNumber := uint32(len(wi.Attempts) + 1)
		attemptID := domain.NewAttemptID()

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkExecutionStarted,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ExecutionStartedPayload{
				AttemptID: attemptID, AttemptNumber: attemptNumber,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Started attempt #%d (%s) on %s\n", attemptNumber, attemptID, workItemDisplayID(wi))
		return nil
	},
}

var workCompleteCmd = &cobra.Command{
	Use:   "complete <id>",
	Short: "Complete the current execution attempt",
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

		if wi.CurrentAttempt == nil {
			return fmt.Errorf("no active attempt on %s", workItemDisplayID(wi))
		}

		summary, _ := cmd.Flags().GetString("summary")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkExecutionCompleted,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ExecutionCompletedPayload{
				AttemptID: *wi.CurrentAttempt, Summary: summary,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Completed attempt %s on %s\n", *wi.CurrentAttempt, workItemDisplayID(wi))
		return nil
	},
}

var workFailCmd = &cobra.Command{
	Use:   "fail <id>",
	Short: "Fail the current execution attempt",
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

		if wi.CurrentAttempt == nil {
			return fmt.Errorf("no active attempt on %s", workItemDisplayID(wi))
		}

		errMsg, _ := cmd.Flags().GetString("error")
		retryable, _ := cmd.Flags().GetBool("retryable")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkExecutionFailed,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ExecutionFailedPayload{
				AttemptID: *wi.CurrentAttempt, Error: errMsg, Retryable: retryable,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Failed attempt %s on %s\n", *wi.CurrentAttempt, workItemDisplayID(wi))
		return nil
	},
}

var workCheckpointCmd = &cobra.Command{
	Use:   "checkpoint <id>",
	Short: "Record a checkpoint on the current attempt",
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

		if wi.CurrentAttempt == nil {
			return fmt.Errorf("no active attempt on %s", workItemDisplayID(wi))
		}

		summary, _ := cmd.Flags().GetString("summary")
		progress, _ := cmd.Flags().GetFloat64("progress")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkCheckpointed,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.CheckpointedPayload{
				AttemptID: *wi.CurrentAttempt, Summary: summary, Progress: progress,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Checkpoint [%.0f%%] %s on %s\n", progress*100, summary, workItemDisplayID(wi))
		return nil
	},
}

var workBlockCmd = &cobra.Command{
	Use:   "block <id>",
	Short: "Mark a work item as blocked",
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

		reason, _ := cmd.Flags().GetString("reason")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkBlocked,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.BlockedPayload{Reason: reason}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Blocked %s: %s\n", workItemDisplayID(wi), reason)
		return nil
	},
}

var workUnblockCmd = &cobra.Command{
	Use:   "unblock <id>",
	Short: "Unblock a work item",
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

		reason, _ := cmd.Flags().GetString("reason")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkUnblocked,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.UnblockedPayload{Reason: reason}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Unblocked %s\n", workItemDisplayID(wi))
		return nil
	},
}

var workObserveCmd = &cobra.Command{
	Use:   "observe <id>",
	Short: "Record an observation",
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

		summary, _ := cmd.Flags().GetString("summary")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkObservationRecorded,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ObservationRecordedPayload{Summary: summary}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Observed: %s\n", summary)
		return nil
	},
}

var workFindingCmd = &cobra.Command{
	Use:   "finding <id>",
	Short: "Record a finding",
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

		statement, _ := cmd.Flags().GetString("statement")
		confidence, _ := cmd.Flags().GetFloat64("confidence")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkFindingRecorded,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.FindingRecordedPayload{
				Statement: statement, Confidence: confidence,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Finding [%.0f%%]: %s\n", confidence*100, statement)
		return nil
	},
}

var workPlanCmd = &cobra.Command{
	Use:   "plan <id>",
	Short: "Propose a plan",
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

		summary, _ := cmd.Flags().GetString("summary")
		plan, _ := cmd.Flags().GetString("plan")

		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkPlanProposed,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.PlanProposedPayload{Plan: plan, Summary: summary}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Plan proposed: %s\n", summary)
		return nil
	},
}

var workHandoffCmd = &cobra.Command{
	Use:   "handoff <id>",
	Short: "Hand off a work item to another actor",
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

		to, _ := cmd.Flags().GetString("to")
		ctxText, _ := cmd.Flags().GetString("context")

		handoffID := domain.NewHandoffID()
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkHandedOff,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.HandedOffPayload{
				HandoffID: handoffID, From: proj.Config.ActorID,
				To: domain.ActorID(to), Context: ctxText,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Handed off %s to %s (%s)\n", workItemDisplayID(wi), to, handoffID)
		return nil
	},
}

var workReviewCmd = &cobra.Command{
	Use:   "review <id>",
	Short: "Request a review",
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

		scope, _ := cmd.Flags().GetString("scope")

		reviewID := domain.NewReviewID()
		heads, _ := proj.DB.GetHeads(ctx, wi.ID)
		event := domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventWorkReviewRequested,
			ParentEventIDs: heads, ActorID: proj.Config.ActorID, Timestamp: time.Now().UTC(),
			Payload: domain.MustMarshalPayload(domain.ReviewRequestedPayload{
				ReviewID: reviewID, Scope: scope,
			}),
		}

		if err := appendAndMaterialize(ctx, proj, wi.ID, event); err != nil {
			return err
		}
		fmt.Printf("Review requested on %s (%s): %s\n", workItemDisplayID(wi), reviewID, scope)
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
}
