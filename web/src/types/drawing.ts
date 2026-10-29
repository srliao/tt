/**
 * TypeScript types mirroring `internal/drawing/types.go`.
 */

/** Listing shape from `GET /drawings` — no scene, so the list stays cheap. */
export interface DrawingSummary {
  id: number;
  name: string;
  /** RFC3339 timestamp. */
  created_at: string;
  /** RFC3339 timestamp. Bumped on every rename or scene save. */
  updated_at: string;
}

/**
 * Full shape from `GET /drawings/:id`. `scene` is an opaque Excalidraw scene
 * (`{elements, appState, files}`); a freshly created drawing has `{}`.
 */
export interface Drawing extends DrawingSummary {
  scene: Record<string, unknown>;
}
