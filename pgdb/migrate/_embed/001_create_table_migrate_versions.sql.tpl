CREATE TABLE IF NOT EXISTS "migrate_versions" (
  "group"   TEXT NOT NULL,
  "version" INTEGER      NOT NULL CHECK ("version" >= 0),
  "created" TIMESTAMPTZ  NOT NULL DEFAULT current_timestamp,

  PRIMARY KEY ("group"),
	CONSTRAINT chk_group_length CHECK (length("group") >= 1 AND length("group") <= 100)
)
