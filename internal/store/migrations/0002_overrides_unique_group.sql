-- One active override per group, not a history log: a later SetOverride
-- call for the same group replaces the earlier one rather than adding a
-- second row that Select would have to disambiguate.
CREATE UNIQUE INDEX idx_overrides_group_id ON overrides (group_id);
