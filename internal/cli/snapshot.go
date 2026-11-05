package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/db"
)

const snapshotContentType = "application/vnd.sqlite3"

// snapshotMeta is the file.db.json sidecar written next to a pulled snapshot.
type snapshotMeta struct {
	Timezone      string `json:"timezone"`
	ServerVersion string `json:"server_version"`
	PulledAt      string `json:"pulled_at"`
}

func newSnapshotCmd(s *session) *cobra.Command {
	return group("snapshot", "Save a copy of the server's database",
		newSnapshotPullCmd(s),
	)
}

func newSnapshotPullCmd(s *session) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "pull -o <file.db>",
		Short: "Download the server's database to a file",
		Long: `Download a consistent copy of the server's database to <file.db> and write
<file.db>.json beside it with the server's time zone and version. Pass the
file to ` + "`plan check --snapshot`" + ` or ` + "`script simulate --snapshot`" + ` to reuse it
instead of fetching a fresh copy each time. The copy is read-only as far as
the server is concerned: nothing is ever written back from it.

Output: {path, meta_path, timezone, server_version, pulled_at}`,
		Example: `  tt-cli snapshot pull -o /tmp/tt.db`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out == "" {
				return usageErr("-o <file.db> is required", nil)
			}
			image, meta, err := s.fetchSnapshot(cmd.Context())
			if err != nil {
				return err
			}
			b, err := json.MarshalIndent(meta, "", "  ")
			if err != nil {
				return err
			}
			metaPath := out + ".json"
			if err := os.WriteFile(out, image, 0o600); err != nil {
				return usageErr("write snapshot: "+err.Error(), map[string]any{"path": out})
			}
			if err := os.WriteFile(metaPath, append(b, '\n'), 0o600); err != nil {
				return usageErr("write snapshot metadata: "+err.Error(), map[string]any{"path": metaPath})
			}
			return s.out(map[string]any{
				"path":           out,
				"meta_path":      metaPath,
				"timezone":       meta.Timezone,
				"server_version": meta.ServerVersion,
				"pulled_at":      meta.PulledAt,
			})
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "file to write (required)")
	return cmd
}

func (s *session) fetchSnapshot(ctx context.Context) ([]byte, snapshotMeta, error) {
	c, err := s.api()
	if err != nil {
		return nil, snapshotMeta{}, err
	}
	image, hdr, err := c.Fetch(ctx, "/snapshot.db", snapshotContentType)
	if err != nil {
		return nil, snapshotMeta{}, err
	}
	return image, snapshotMeta{
		Timezone:      hdr.Get("X-TT-Timezone"),
		ServerVersion: hdr.Get("X-TT-Version"),
		PulledAt:      s.Now().UTC().Format(time.RFC3339),
	}, nil
}

// openSnapshot loads a snapshot into in-memory SQLite: the saved file at
// path (with its .json sidecar), or a fresh copy from the server when path
// is empty. The caller closes the store.
func (s *session) openSnapshot(ctx context.Context, path string) (*db.Store, snapshotMeta, error) {
	var (
		image []byte
		meta  snapshotMeta
		err   error
	)
	if path == "" {
		image, meta, err = s.fetchSnapshot(ctx)
		if err != nil {
			return nil, meta, err
		}
	} else {
		image, err = os.ReadFile(path)
		if err != nil {
			return nil, meta, usageErr("read snapshot: "+err.Error(), map[string]any{"path": path})
		}
		raw, err := os.ReadFile(path + ".json")
		if err != nil {
			return nil, meta, usageErr("read snapshot metadata (pull snapshots with `tt-cli snapshot pull`): "+err.Error(),
				map[string]any{"path": path + ".json"})
		}
		if err := json.Unmarshal(raw, &meta); err != nil {
			return nil, meta, usageErr("parse snapshot metadata: "+err.Error(), map[string]any{"path": path + ".json"})
		}
	}

	store, err := db.OpenSnapshot(ctx, image)
	var vm *db.VersionMismatchError
	if errors.As(err, &vm) {
		side := "upgrade the tt server (or pull a newer snapshot)"
		if vm.Snapshot > vm.Latest {
			side = "upgrade tt-cli"
		}
		return nil, meta, &Error{
			Code:    "version_mismatch",
			Message: fmt.Sprintf("snapshot schema is version %d but tt-cli expects %d: %s", vm.Snapshot, vm.Latest, side),
			Details: map[string]any{"snapshot_version": vm.Snapshot, "cli_version": vm.Latest, "server_version": meta.ServerVersion},
			Exit:    ExitRemote,
		}
	}
	if err != nil {
		return nil, meta, validationErr("load snapshot: "+err.Error(), map[string]any{"path": path})
	}
	return store, meta, nil
}
