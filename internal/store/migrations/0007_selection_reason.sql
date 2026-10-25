-- Why the group's current pick was made (rules.Select's explanation, or the
-- manual override), so the library browser can show it (spec §5: every
-- selection explains itself).
ALTER TABLE game_groups ADD COLUMN selection_reason TEXT NOT NULL DEFAULT '';
