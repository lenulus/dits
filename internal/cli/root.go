package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/workops"
	"github.com/spf13/cobra"
)

// cliLogger is configured by the persistent flags on rootCmd. CLI shims
// pass it into workops.SetLogger so the same observability schema the
// MCP server emits is available from `dits` invocations as well.
var cliLogger *slog.Logger = logging.Discard()

var (
	flagLogLevel  string
	flagLogFormat string
)

// loadProject discovers and opens the project from cwd. Thin delegate to
// workops.Open so non-work CLI commands share the same discovery path as
// the work CLI and the MCP server.
func loadProject() (*project.Project, error) {
	w, err := workops.Open()
	if err != nil {
		return nil, err
	}
	w.SetLogger(cliLogger)
	return w.Proj, nil
}

// loadMeta and resolveWorkItem delegate to workops so the meta-loading and
// reference resolution logic lives in exactly one place.
func loadMeta(ctx context.Context, proj *project.Project) (*domain.MetaConfig, error) {
	w := &workops.WorkOps{Proj: proj}
	w.SetLogger(cliLogger)
	return w.LoadMeta(ctx)
}

func signEvent(proj *project.Project, e *domain.Event) error {
	w := &workops.WorkOps{Proj: proj}
	w.SetLogger(cliLogger)
	return w.SignEvent(e)
}

func resolveWorkItem(ctx context.Context, proj *project.Project, ref string) (*domain.WorkItem, error) {
	w := &workops.WorkOps{Proj: proj}
	w.SetLogger(cliLogger)
	return w.ResolveWorkItem(ctx, ref)
}

var rootCmd = &cobra.Command{
	Use:   "dits",
	Short: "Distributed Issue Tracking System",
	Long:  "DITS is a local-first, event-sourced distributed issue tracker.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		level, err := logging.Parse(flagLogLevel)
		if err != nil {
			return err
		}
		cliLogger = logging.New(os.Stderr, level, flagLogFormat)
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagLogLevel, "log-level", "info", "log level: error, warn, info, debug, trace")
	rootCmd.PersistentFlags().StringVar(&flagLogFormat, "log-format", "text", "log format: text or json")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(workCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(remoteCmd)
	rootCmd.AddCommand(metaCmd)
	rootCmd.AddCommand(identityCmd)
}
