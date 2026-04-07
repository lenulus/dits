package workops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/lenulus/pf/internal/domain"
	dsync "github.com/lenulus/pf/internal/sync"
)

const defaultServerNodeID = domain.NodeID("server")

// SyncResult summarizes the outcome of a Sync call.
type SyncResult struct {
	ServerURL       string `json:"server_url"`
	Pushed          int    `json:"pushed_events"`
	Pulled          int    `json:"pulled_events"`
	NewSharedIDs    int    `json:"new_shared_ids"`
	BlobsUploaded   int    `json:"blobs_uploaded"`
	BlobsDownloaded int    `json:"blobs_downloaded"`
}

// Sync runs one push/pull cycle against the configured (or supplied) remote.
// If serverURL is empty, the project's configured remote is used. If neither
// is set, an error is returned. The first explicit URL is persisted to config
// (matching the CLI behavior).
func (w *WorkOps) Sync(ctx context.Context, serverURL string) (*SyncResult, error) {
	proj := w.Proj

	if serverURL == "" {
		serverURL = proj.Config.ServerURL
	}
	if serverURL == "" {
		return nil, fmt.Errorf("no server URL configured; use --server or run 'dits remote set <url>'")
	}
	if proj.Config.ServerURL == "" {
		proj.Config.ServerURL = serverURL
		if err := proj.SaveConfig(); err != nil {
			return nil, fmt.Errorf("saving config: %w", err)
		}
	}

	engine := dsync.NewEngine(proj.DB)
	req, err := engine.BuildSyncRequest(ctx, proj.Config.NodeID, proj.Config.ProjectKey, defaultServerNodeID)
	if err != nil {
		return nil, fmt.Errorf("building sync request: %w", err)
	}
	if proj.Identity != nil {
		req.ActorID = proj.Identity.ActorID
		req.PublicKey = proj.Identity.PublicKey
	}

	client := dsync.NewClient(serverURL)
	resp, err := client.Sync(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sync failed: %w", err)
	}
	if err := engine.ApplySync(ctx, resp, defaultServerNodeID); err != nil {
		return nil, fmt.Errorf("applying sync response: %w", err)
	}

	result := &SyncResult{
		ServerURL:    serverURL,
		Pushed:       len(req.Events),
		Pulled:       len(resp.Events),
		NewSharedIDs: len(resp.SharedIDs),
	}

	// Push blobs the server is missing.
	if pushed := collectArtifactHashes(req.Events); len(pushed) > 0 {
		missing, err := checkMissingBlobs(ctx, serverURL, pushed)
		if err != nil {
			return result, fmt.Errorf("checking blobs: %w", err)
		}
		for _, h := range missing {
			rc, err := proj.Blobs.Get(ctx, h)
			if err != nil {
				continue
			}
			if err := uploadBlob(ctx, serverURL, h, rc); err != nil {
				rc.Close()
				return result, fmt.Errorf("uploading blob %s: %w", h, err)
			}
			rc.Close()
			result.BlobsUploaded++
		}
	}

	// Pull blobs referenced by new events.
	if pulled := collectArtifactHashes(resp.Events); len(pulled) > 0 {
		for _, h := range pulled {
			has, _ := proj.Blobs.Has(ctx, h)
			if has {
				continue
			}
			if err := downloadBlob(ctx, serverURL, h, proj.Blobs); err != nil {
				w.Logger().WarnContext(ctx, "blob download failed (continuing)",
					slog.String("hash", h),
					slog.Any("err", err),
				)
				continue
			}
			result.BlobsDownloaded++
		}
	}

	w.Logger().InfoContext(ctx, "sync completed",
		slog.String("server_url", serverURL),
		slog.Int("pushed", result.Pushed),
		slog.Int("pulled", result.Pulled),
		slog.Int("blobs_uploaded", result.BlobsUploaded),
		slog.Int("blobs_downloaded", result.BlobsDownloaded),
	)
	return result, nil
}

func collectArtifactHashes(events []domain.Event) []string {
	seen := make(map[string]struct{})
	var hashes []string
	for _, e := range events {
		switch e.Type {
		case domain.EventWorkArtifactAdded:
			var p domain.ArtifactAddedPayload
			if err := json.Unmarshal(e.Payload, &p); err == nil {
				if _, ok := seen[p.ContentHash]; !ok {
					seen[p.ContentHash] = struct{}{}
					hashes = append(hashes, p.ContentHash)
				}
			}
		case domain.EventWorkEvidenceAttached:
			var p domain.EvidenceAttachedPayload
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

func downloadBlob(_ context.Context, serverURL, hash string, store interface {
	Put(context.Context, string, io.Reader) error
}) error {
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
