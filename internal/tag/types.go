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

// TagWithCount is the listing shape returned by Service.ListWithCounts. Count
// is the number of distinct tasks referencing the tag through task_tags.
// Mirrored on the frontend at `web/src/types/tag.ts`.
type TagWithCount struct {
	Tag
	Count int64 `json:"count"`
}
