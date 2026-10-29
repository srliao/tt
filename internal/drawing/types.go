// Package drawing provides the domain service for Excalidraw canvases.
//
// A drawing is a name plus an opaque Excalidraw scene blob. The service
// validates that the scene is a JSON object but never interprets it; the
// SPA owns the scene format.
package drawing

import (
	"encoding/json"
	"time"
)

// Summary is the listing shape: everything except the (potentially large)
// scene. Mirrored on the frontend at `web/src/types/drawing.ts`.
type Summary struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Drawing is the full shape returned by Get and the mutators.
type Drawing struct {
	Summary
	Scene json.RawMessage `json:"scene"`
}

// UpdateInput is a partial update: nil fields are left unchanged.
type UpdateInput struct {
	Name  *string
	Scene json.RawMessage
}
