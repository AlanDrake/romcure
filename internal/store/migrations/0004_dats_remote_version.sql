-- Remote provenance for auto-downloaded DATs (docs/redump-integration-plan.md):
-- which upstream slug a DAT came from and the version string the upstream
-- catalog showed, so an unchanged remote version skips the download.
ALTER TABLE dats ADD COLUMN remote_source  TEXT;
ALTER TABLE dats ADD COLUMN remote_slug    TEXT;
ALTER TABLE dats ADD COLUMN remote_version TEXT;
