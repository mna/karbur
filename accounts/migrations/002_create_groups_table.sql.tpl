CREATE TABLE "accounts_groups" (
  "id"       SERIAL NOT NULL,
  "name"     TEXT NOT NULL CHECK (length("name") <= 128),
  "created"  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY ("id"),
  UNIQUE ("name")
);
