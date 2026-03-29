package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

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

		// Sync blobs: upload local blobs the server is missing.
		pushedHashes := collectAttachmentHashes(req.Events)
		if len(pushedHashes) > 0 {
			missing, err := checkMissingBlobs(ctx, serverURL, pushedHashes)
			if err != nil {
				return fmt.Errorf("checking blobs: %w", err)
			}
			if len(missing) > 0 {
				fmt.Printf("  Uploading %d blobs...\n", len(missing))
				for _, h := range missing {
					rc, err := proj.Blobs.Get(ctx, h)
					if err != nil {
						fmt.Fprintf(os.Stderr, "  warning: blob %s not found locally, skipping\n", h)
						continue
					}
					if err := uploadBlob(ctx, serverURL, h, rc); err != nil {
						rc.Close()
						return fmt.Errorf("uploading blob %s: %w", h, err)
					}
					rc.Close()
				}
			}
		}

		// Sync blobs: download blobs from pulled events we don't have locally.
		pulledHashes := collectAttachmentHashes(resp.Events)
		if len(pulledHashes) > 0 {
			var needed []string
			for _, h := range pulledHashes {
				has, _ := proj.Blobs.Has(ctx, h)
				if !has {
					needed = append(needed, h)
				}
			}
			if len(needed) > 0 {
				fmt.Printf("  Downloading %d blobs...\n", len(needed))
				for _, h := range needed {
					if err := downloadBlob(ctx, serverURL, h, proj.Blobs); err != nil {
						fmt.Fprintf(os.Stderr, "  warning: failed to download blob %s: %v\n", h, err)
					}
				}
			}
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

// --- Blob sync helpers ---

func collectAttachmentHashes(events []domain.Event) []string {
	seen := make(map[string]struct{})
	var hashes []string
	for _, e := range events {
		if e.Type == domain.EventAttachmentAdded {
			var p domain.AttachmentAddedPayload
			if err := json.Unmarshal(e.Payload, &p); err == nil {
				if _, ok := seen[p.ContentHash]; !ok {
					seen[p.ContentHash] = struct{}{}
					hashes = append(hashes, p.ContentHash)
				}
			}
		}
	}
	return hashes
}

type blobCheckReq struct {
	Hashes []string `json:"hashes"`
}

type blobCheckResp struct {
	Present []string `json:"present"`
	Missing []string `json:"missing"`
}

func checkMissingBlobs(_ context.Context, serverURL string, hashes []string) ([]string, error) {
	body, _ := json.Marshal(blobCheckReq{Hashes: hashes})
	resp, err := http.Post(serverURL+"/api/v1/blobs/check", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result blobCheckResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Missing, nil
}

func uploadBlob(_ context.Context, serverURL, hash string, r io.Reader) error {
	req, err := http.NewRequest(http.MethodPut, serverURL+"/api/v1/blobs/"+hash, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func downloadBlob(_ context.Context, serverURL, hash string, store interface{ Put(context.Context, string, io.Reader) error }) error {
	resp, err := http.Get(serverURL + "/api/v1/blobs/" + hash)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	return store.Put(context.Background(), hash, resp.Body)
}
