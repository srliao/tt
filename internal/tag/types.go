// Package tag provides the domain service for tag management.
//
// Tags are name-only labels associated with tasks. The service exposes CRUD
// plus a Resolve helper that converts free-text tag names (with optional
// auto-creation) into the ids used by the task service for attachment.
package tag

import "time"

// Tag is the domain-layer representation of a row in the tags table.
type Tag struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
