/**
 * TypeScript types mirroring `internal/tag/types.go`.
 */

export interface Tag {
  id: number;
  name: string;
  /**
   * OKLCH hue angle in degrees, snapped to the 12-step palette (multiples
   * of 30, range [0, 330]). Assigned at creation time by the backend's
   * least-used-hue rule. When threaded into `tagColor()`/`tagColorDark()`
   * it short-circuits the hash so two unrelated tags can't collide.
   */
  color_hue: number;
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
