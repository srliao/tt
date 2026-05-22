/**
 * Deterministic per-tag colors.
 *
 * Hashes the lowercased tag name with FNV-1a, then picks one of 12 evenly
 * spaced hues on the OKLCH color wheel. Returns three coordinated colors
 * (background, foreground, dot) for use in pill / chip / dot UIs.
 *
 * The two variants — `tagColor` for light themes, `tagColorDark` for dark —
 * keep the same hue but swap the lightness / chroma to maintain WCAG-ish
 * contrast against the surface color.
 *
 * Stability guarantees:
 *   - same name → same color across reloads, components, and pages
 *   - case-insensitive (`tagColor("Foo") === tagColor("foo")`)
 *   - hash is pure JS, no DOM / crypto dependency, safe in tests
 *
 * The phase 0 design doc declares this module the single source of truth for
 * tag color; every chip / dot / filter row in later phases reaches through
 * <TagChip> rather than re-deriving hues locally.
 */

const HUES = [0, 30, 60, 90, 120, 150, 180, 210, 240, 270, 300, 330];

/**
 * 32-bit FNV-1a hash. Uses `Math.imul` so the multiplication stays inside
 * the int32 range — a plain `*` would lose precision past 2^53 and produce
 * different hues on different inputs.
 */
function hash(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h = Math.imul(h ^ s.charCodeAt(i), 16777619);
  }
  return h >>> 0;
}

/** Color palette used by light themes. */
export function tagColor(name: string): { bg: string; fg: string; dot: string } {
  const hue = HUES[hash(name.toLowerCase()) % HUES.length];
  return {
    bg: `oklch(0.94 0.04 ${hue})`,
    fg: `oklch(0.40 0.12 ${hue})`,
    dot: `oklch(0.55 0.14 ${hue})`,
  };
}

/** Color palette used by dark themes — same hue, inverted lightness. */
export function tagColorDark(name: string): { bg: string; fg: string; dot: string } {
  const hue = HUES[hash(name.toLowerCase()) % HUES.length];
  return {
    bg: `oklch(0.28 0.05 ${hue})`,
    fg: `oklch(0.85 0.08 ${hue})`,
    dot: `oklch(0.70 0.14 ${hue})`,
  };
}
