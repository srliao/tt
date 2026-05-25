-- +goose Up

-- Per-tag color persistence. Previously colors were derived client-side by
-- hashing the tag name into a 12-hue palette, which guaranteed collisions
-- past 12 tags (and made them likely much earlier via the birthday paradox).
--
-- The hue is the OKLCH hue angle in degrees, snapped to a 12-step palette
-- (0, 30, 60, ... 330). NOT NULL with default 0 lets us add the column
-- without a backfill window; the UPDATE below immediately spreads any
-- existing rows across the palette so users with colliding tags see a
-- visible improvement on first start after the upgrade.
--
-- New tag creates compute their own hue via the least-used-hue rule in
-- internal/tag/service.go; this migration only seeds pre-existing rows.
ALTER TABLE tags ADD COLUMN color_hue INTEGER NOT NULL DEFAULT 0;

UPDATE tags
SET color_hue = ((id - 1) % 12) * 30;

-- +goose Down

ALTER TABLE tags DROP COLUMN color_hue;
