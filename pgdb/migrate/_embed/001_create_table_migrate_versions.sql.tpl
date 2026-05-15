CREATE TABLE IF NOT EXISTS "migrate_versions" (
  "group"   VARCHAR(100) NOT NULL,
  "version" INTEGER      NOT NULL CHECK ("version" >= 0),
  "created" TIMESTAMPTZ  NOT NULL DEFAULT current_timestamp,

  PRIMARY KEY ("group")
)
