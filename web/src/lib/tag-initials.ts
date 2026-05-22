/**
 * Compact-row tag glyph initials.
 *
 * Given the visible tag set, return a map of tag-name → 1- or 2-character
 * display initial that disambiguates within that set. The result is stable
 * for a given input — render-derived only, never store it.
 *
 * Rules:
 *  - Default: first letter, uppercased.
 *  - If two tags share the first letter: use first two letters
 *    (first upper, second lower).
 *  - If two tags share the first two letters: fall back to first letter +
 *    1-based index in sorted order (B1, B2, …).
 *  - Comparison is case-insensitive; output preserves the casing rules above.
 *
 * Designed for `<TagGlyph>` in `web/src/components/ui/tag-glyph.tsx`. Full
 * tag chips (sidebar, modal, palette) keep using `<TagChip>` from
 * `tag-chip.tsx` — the glyph is row-specific.
 */
export function buildInitialMap(names: string[]): Map<string, string> {
  const out = new Map<string, string>();
  const byFirst = new Map<string, string[]>();
  for (const n of names) {
    const k = (n[0] ?? '?').toUpperCase();
    if (!byFirst.has(k)) byFirst.set(k, []);
    byFirst.get(k)?.push(n);
  }
  for (const [first, group] of byFirst) {
    if (group.length === 1) {
      const n = group[0];
      const display = n.length === 0 ? '?' : first;
      out.set(n, display);
      continue;
    }
    // Try second-letter disambiguation: distinct second letters → use two
    // letters; any duplicate second letter forces the index fallback.
    const bySecond = new Map<string, string[]>();
    for (const n of group) {
      const k = (n[1] ?? '').toLowerCase();
      if (!bySecond.has(k)) bySecond.set(k, []);
      bySecond.get(k)?.push(n);
    }
    let needsIndex = false;
    for (const sub of bySecond.values()) {
      if (sub.length > 1) {
        needsIndex = true;
        break;
      }
    }
    if (!needsIndex) {
      for (const n of group) {
        const display = (n[0] ?? '?').toUpperCase() + (n[1] ?? '').toLowerCase();
        out.set(n, display);
      }
    } else {
      const sorted = [...group].sort((a, b) => a.localeCompare(b));
      sorted.forEach((n, i) => {
        out.set(n, `${first}${i + 1}`);
      });
    }
  }
  return out;
}
