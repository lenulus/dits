package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/lenulus/pf/internal/crypto"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/project"
	"github.com/spf13/cobra"
)

// loadProject mirrors the legacy CLI helper: discover the project root from
// cwd and open it. Most CLI commands still use this directly; work commands
// go through internal/workops instead.
func loadProject() (*project.Project, error) {
	root, err := project.FindRoot()
	if err != nil {
		return nil, err
	}
	return project.Load(root)
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

func signEvent(proj *project.Project, e *domain.Event) error {
	if k := proj.PrivKey(); k != nil {
		return crypto.SignEvent(e, k)
	}
	return nil
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

var rootCmd = &cobra.Command{
	Use:   "dits",
	Short: "Distributed Issue Tracking System",
	Long:  "DITS is a local-first, event-sourced distributed issue tracker.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(workCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(remoteCmd)
	rootCmd.AddCommand(metaCmd)
	rootCmd.AddCommand(identityCmd)
}
