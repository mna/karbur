CREATE TABLE "accounts_accounts" (
  "id"       SERIAL NOT NULL,
  -- see https://stackoverflow.com/a/574698/1094941
  "email"    TEXT NOT NULL CHECK (length("email") >= 3 AND length("email") <= 254),
  "password" TEXT NOT NULL CHECK (length("password") <= 1024),
  "verified" TIMESTAMPTZ NULL,
  "created"  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY ("id"),
  UNIQUE ("email")
);
