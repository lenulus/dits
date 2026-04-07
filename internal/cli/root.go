package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/workops"
	"github.com/spf13/cobra"
)

// loadProject discovers and opens the project from cwd. Thin delegate to
// workops.Open so non-work CLI commands share the same discovery path as
// the work CLI and the MCP server.
func loadProject() (*project.Project, error) {
	w, err := workops.Open()
	if err != nil {
		return nil, err
	}
	return w.Proj, nil
}

// loadMeta and resolveWorkItem delegate to workops so the meta-loading and
// reference resolution logic lives in exactly one place.
func loadMeta(ctx context.Context, proj *project.Project) (*domain.MetaConfig, error) {
	return (&workops.WorkOps{Proj: proj}).LoadMeta(ctx)
}

func signEvent(proj *project.Project, e *domain.Event) error {
	return (&workops.WorkOps{Proj: proj}).SignEvent(e)
}

func resolveWorkItem(ctx context.Context, proj *project.Project, ref string) (*domain.WorkItem, error) {
	return (&workops.WorkOps{Proj: proj}).ResolveWorkItem(ctx, ref)
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
