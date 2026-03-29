package cli

import (
	"context"
	"fmt"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store"
	"github.com/spf13/cobra"
)

var metaCmd = &cobra.Command{
	Use:   "meta",
	Short: "Manage project meta configuration (labels, workflows, types)",
}

var metaShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current meta configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		meta, err := proj.DB.GetCurrentMeta(context.Background())
		if err != nil {
			return err
		}
		if meta == nil {
			fmt.Println("No meta configuration.")
			return nil
		}

		fmt.Printf("Project:  %s\n", meta.ProjectKey)
		fmt.Printf("Version:  %d\n", meta.Version)

		fmt.Printf("\nLabels (%d):\n", len(meta.Labels))
		for _, l := range meta.Labels {
			color := ""
			if l.Color != "" {
				color = fmt.Sprintf(" [%s]", l.Color)
			}
			fmt.Printf("  %-20s %s%s\n", l.Slug, l.Name, color)
		}
		if len(meta.Labels) == 0 {
			fmt.Println("  (none)")
		}

		fmt.Printf("\nWorkflows (%d):\n", len(meta.Workflows))
		for _, w := range meta.Workflows {
			fmt.Printf("  %s (%s):\n", w.Name, w.Slug)
			for _, s := range w.Statuses {
				fmt.Printf("    %-20s [%s]\n", s.Slug, s.Category)
			}
		}

		fmt.Printf("\nIssue Types (%d):\n", len(meta.IssueTypes))
		for _, t := range meta.IssueTypes {
			fmt.Printf("  %-20s workflow: %s\n", t.Slug, t.WorkflowSlug)
		}

		fmt.Printf("\nPriorities: %v\n", meta.Priorities)
		return nil
	},
}

// --- Label commands ---

var metaLabelCmd = &cobra.Command{
	Use:   "label",
	Short: "Manage labels",
}

var metaLabelAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a label",
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		name, _ := cmd.Flags().GetString("name")
		color, _ := cmd.Flags().GetString("color")

		if slug == "" {
			return fmt.Errorf("--slug is required")
		}
		if name == "" {
			name = slug
		}

		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		meta, err := proj.DB.GetCurrentMeta(ctx)
		if err != nil {
			return err
		}
		if meta == nil {
			return fmt.Errorf("no meta configuration found")
		}

		prevVersion := meta.Version
		if err := meta.AddLabel(domain.Label{Slug: slug, Name: name, Color: color}); err != nil {
			return err
		}

		if err := proj.DB.SaveMetaIfVersion(ctx, meta, prevVersion); err != nil {
			if err == store.ErrMetaConflict {
				return fmt.Errorf("meta was modified concurrently; please retry")
			}
			return err
		}

		fmt.Printf("Added label %q (version %d)\n", slug, meta.Version)
		return nil
	},
}

var metaLabelRemoveCmd = &cobra.Command{
	Use:   "remove <slug>",
	Short: "Remove a label",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		meta, err := proj.DB.GetCurrentMeta(ctx)
		if err != nil {
			return err
		}
		if meta == nil {
			return fmt.Errorf("no meta configuration found")
		}

		prevVersion := meta.Version
		if err := meta.RemoveLabel(args[0]); err != nil {
			return err
		}

		if err := proj.DB.SaveMetaIfVersion(ctx, meta, prevVersion); err != nil {
			if err == store.ErrMetaConflict {
				return fmt.Errorf("meta was modified concurrently; please retry")
			}
			return err
		}

		fmt.Printf("Removed label %q (version %d)\n", args[0], meta.Version)
		return nil
	},
}

var metaLabelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all labels",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		meta, err := proj.DB.GetCurrentMeta(context.Background())
		if err != nil {
			return err
		}
		if meta == nil || len(meta.Labels) == 0 {
			fmt.Println("No labels defined.")
			return nil
		}
		for _, l := range meta.Labels {
			color := ""
			if l.Color != "" {
				color = fmt.Sprintf(" [%s]", l.Color)
			}
			fmt.Printf("%-20s %s%s\n", l.Slug, l.Name, color)
		}
		return nil
	},
}

// --- Type commands ---

var metaTypeCmd = &cobra.Command{
	Use:   "type",
	Short: "Manage issue types",
}

var metaTypeAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an issue type",
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		name, _ := cmd.Flags().GetString("name")
		workflow, _ := cmd.Flags().GetString("workflow")

		if slug == "" {
			return fmt.Errorf("--slug is required")
		}
		if name == "" {
			name = slug
		}
		if workflow == "" {
			workflow = "default"
		}

		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		ctx := context.Background()
		meta, err := proj.DB.GetCurrentMeta(ctx)
		if err != nil {
			return err
		}
		if meta == nil {
			return fmt.Errorf("no meta configuration found")
		}

		prevVersion := meta.Version
		if err := meta.AddIssueType(domain.IssueType{Slug: slug, Name: name, WorkflowSlug: workflow}); err != nil {
			return err
		}

		if err := proj.DB.SaveMetaIfVersion(ctx, meta, prevVersion); err != nil {
			if err == store.ErrMetaConflict {
				return fmt.Errorf("meta was modified concurrently; please retry")
			}
			return err
		}

		fmt.Printf("Added issue type %q (version %d)\n", slug, meta.Version)
		return nil
	},
}

var metaTypeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all issue types",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		meta, err := proj.DB.GetCurrentMeta(context.Background())
		if err != nil {
			return err
		}
		if meta == nil || len(meta.IssueTypes) == 0 {
			fmt.Println("No issue types defined.")
			return nil
		}
		for _, t := range meta.IssueTypes {
			fmt.Printf("%-20s %-20s workflow: %s\n", t.Slug, t.Name, t.WorkflowSlug)
		}
		return nil
	},
}

// --- Workflow commands ---

var metaWorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage workflows",
}

var metaWorkflowShowCmd = &cobra.Command{
	Use:   "show [slug]",
	Short: "Show workflow details",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		meta, err := proj.DB.GetCurrentMeta(context.Background())
		if err != nil {
			return err
		}
		if meta == nil {
			return fmt.Errorf("no meta configuration found")
		}

		slug := "default"
		if len(args) > 0 {
			slug = args[0]
		}

		for _, w := range meta.Workflows {
			if w.Slug == slug {
				fmt.Printf("Workflow: %s (%s)\n\n", w.Name, w.Slug)
				fmt.Println("Statuses:")
				for _, s := range w.Statuses {
					fmt.Printf("  %-20s %-20s [%s]\n", s.Slug, s.Name, s.Category)
				}
				fmt.Println("\nTransitions:")
				for _, t := range w.Transitions {
					fmt.Printf("  %s -> %s\n", t.From, t.To)
				}
				return nil
			}
		}
		return fmt.Errorf("workflow %q not found", slug)
	},
}

var metaWorkflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workflows",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		meta, err := proj.DB.GetCurrentMeta(context.Background())
		if err != nil {
			return err
		}
		if meta == nil || len(meta.Workflows) == 0 {
			fmt.Println("No workflows defined.")
			return nil
		}
		for _, w := range meta.Workflows {
			statuses := make([]string, len(w.Statuses))
			for i, s := range w.Statuses {
				statuses[i] = s.Slug
			}
			fmt.Printf("%-20s %s\n", w.Slug, w.Name)
		}
		return nil
	},
}

func init() {
	metaCmd.AddCommand(metaShowCmd)
	metaCmd.AddCommand(metaLabelCmd)
	metaCmd.AddCommand(metaTypeCmd)
	metaCmd.AddCommand(metaWorkflowCmd)

	metaLabelCmd.AddCommand(metaLabelAddCmd)
	metaLabelCmd.AddCommand(metaLabelRemoveCmd)
	metaLabelCmd.AddCommand(metaLabelListCmd)

	metaTypeCmd.AddCommand(metaTypeAddCmd)
	metaTypeCmd.AddCommand(metaTypeListCmd)

	metaWorkflowCmd.AddCommand(metaWorkflowShowCmd)
	metaWorkflowCmd.AddCommand(metaWorkflowListCmd)

	metaLabelAddCmd.Flags().String("slug", "", "Label slug (required)")
	metaLabelAddCmd.Flags().String("name", "", "Label display name")
	metaLabelAddCmd.Flags().String("color", "", "Label color (hex)")
	metaLabelAddCmd.MarkFlagRequired("slug")

	metaTypeAddCmd.Flags().String("slug", "", "Type slug (required)")
	metaTypeAddCmd.Flags().String("name", "", "Type display name")
	metaTypeAddCmd.Flags().String("workflow", "default", "Workflow slug")
	metaTypeAddCmd.MarkFlagRequired("slug")
}
