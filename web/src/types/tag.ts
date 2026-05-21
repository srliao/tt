/**
 * TypeScript types mirroring `internal/tag/types.go`.
 */

export interface Tag {
  id: number;
  name: string;
  /** RFC3339 timestamp. */
  created_at: string;
}
