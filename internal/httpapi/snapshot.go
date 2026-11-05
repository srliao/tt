package httpapi

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// handleSnapshot streams a consistent copy of the database for tt-cli to
// load into in-memory SQLite. The copy is staged in a private temp dir that
// is removed however the response ends, including client disconnects.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.snapshotter == nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "snapshot not configured", nil)
		return
	}

	// fail logs a failed step and answers 500 without leaking its detail.
	fail := func(step string, err error) {
		s.logger.Error("snapshot: "+step, "err", err)
		writeError(w, http.StatusInternalServerError, CodeInternal, "snapshot failed", nil)
	}

	dir, err := os.MkdirTemp(s.snapshotDir, "tt-snapshot-*")
	if err != nil {
		fail("create temp dir", err)
		return
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			s.logger.Error("snapshot: remove temp dir", "dir", dir, "err", err)
		}
	}()

	path := filepath.Join(dir, "snapshot.db")
	if err := s.snapshotter.VacuumInto(r.Context(), path); err != nil {
		fail("vacuum", err)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		fail("open copy", err)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		fail("stat copy", err)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/vnd.sqlite3")
	h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	h.Set("X-TT-Timezone", s.timezone)
	h.Set("X-TT-Version", s.version)
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		s.logger.Warn("snapshot: stream copy", "err", err)
	}
}
