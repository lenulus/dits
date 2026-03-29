package cli

import (
	"context"
	"fmt"

	"github.com/lenulus/pf/internal/domain"
	dsync "github.com/lenulus/pf/internal/sync"
	"github.com/spf13/cobra"
)

const defaultServerNodeID = domain.NodeID("server")

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync with remote server",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")

		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		// Resolve server URL: flag > config.
		if serverURL == "" {
			serverURL = proj.Config.ServerURL
		}
		if serverURL == "" {
			return fmt.Errorf("no server URL configured; use --server or run 'dits remote set <url>'")
		}

		// Save server URL to config if provided via flag.
		if proj.Config.ServerURL == "" {
			proj.Config.ServerURL = serverURL
			if err := proj.SaveConfig(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
		}

		ctx := context.Background()
		engine := dsync.NewEngine(proj.DB)

		// Build sync request.
		req, err := engine.BuildSyncRequest(ctx, proj.Config.NodeID, proj.Config.ProjectKey, defaultServerNodeID)
		if err != nil {
			return fmt.Errorf("building sync request: %w", err)
		}

		fmt.Printf("Syncing with %s...\n", serverURL)
		fmt.Printf("  Pushing %d events\n", len(req.Events))

		// Send to server.
		client := dsync.NewClient(serverURL)
		resp, err := client.Sync(ctx, req)
		if err != nil {
			return fmt.Errorf("sync failed: %w", err)
		}

		// Apply response.
		if err := engine.ApplySync(ctx, resp, defaultServerNodeID); err != nil {
			return fmt.Errorf("applying sync response: %w", err)
		}

		fmt.Printf("  Pulled %d events\n", len(resp.Events))
		if len(resp.SharedIDs) > 0 {
			fmt.Printf("  %d new shared IDs assigned\n", len(resp.SharedIDs))
		}
		fmt.Println("Sync complete.")
		return nil
	},
}

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote server",
}

var remoteSetCmd = &cobra.Command{
	Use:   "set <url>",
	Short: "Set remote server URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		proj.Config.ServerURL = args[0]
		if err := proj.SaveConfig(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Remote set to %s\n", args[0])
		return nil
	},
}

var remoteShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show remote server URL",
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := loadProject()
		if err != nil {
			return err
		}
		defer proj.DB.Close()

		if proj.Config.ServerURL == "" {
			fmt.Println("No remote configured.")
		} else {
			fmt.Println(proj.Config.ServerURL)
		}
		return nil
	},
}

func init() {
	syncCmd.Flags().StringP("server", "s", "", "Server URL (overrides config)")

	remoteCmd.AddCommand(remoteSetCmd)
	remoteCmd.AddCommand(remoteShowCmd)
}
