package cli

import (
	"context"
	"fmt"

	"github.com/lenulus/pf/internal/workops"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync with remote server",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")

		w, err := workops.Open()
		if err != nil {
			return err
		}
		w.SetLogger(cliLogger)
		defer w.Shutdown()

		fmt.Printf("Syncing with %s...\n", firstNonEmpty(serverURL, w.Proj.Config.ServerURL))

		res, err := w.Sync(context.Background(), serverURL)
		if err != nil {
			return err
		}

		fmt.Printf("  Pushed %d events\n", res.Pushed)
		fmt.Printf("  Pulled %d events\n", res.Pulled)
		if res.NewSharedIDs > 0 {
			fmt.Printf("  %d new shared IDs assigned\n", res.NewSharedIDs)
		}
		if res.BlobsUploaded > 0 {
			fmt.Printf("  Uploaded %d blobs\n", res.BlobsUploaded)
		}
		if res.BlobsDownloaded > 0 {
			fmt.Printf("  Downloaded %d blobs\n", res.BlobsDownloaded)
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

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
