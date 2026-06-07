CREATE TABLE "accounts_groups" (
  "id"       SERIAL NOT NULL,
  "name"     TEXT NOT NULL,
  "created"  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY ("id"),
  CONSTRAINT uidx_groups_name UNIQUE ("name"),
  CONSTRAINT chk_name_length CHECK (length("name") >= 2 AND length("name") <= 128)
);
