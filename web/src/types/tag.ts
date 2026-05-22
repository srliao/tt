/**
 * TypeScript types mirroring `internal/tag/types.go`.
 */

export interface Tag {
  id: number;
  name: string;
  /** RFC3339 timestamp. */
  created_at: string;
}

/**
 * Tag plus an aggregate of how many tasks reference it. Returned by
 * `GET /tags?counts=1` (see `internal/httpapi/tags.go`). Used by phase 0+
 * to render counts in filter sidebars and command palette entries.
 */
export interface TagWithCount extends Tag {
  count: number;
}
