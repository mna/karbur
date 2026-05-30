CREATE TABLE "accounts_accounts" (
  "id"       SERIAL NOT NULL,
  -- see https://stackoverflow.com/a/574698/1094941
  "email"    TEXT NOT NULL,
  "password" TEXT NOT NULL,
  "verified" TIMESTAMPTZ NULL,
  "created"  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY ("id"),
  CONSTRAINT uidx_accounts_email UNIQUE ("email"),
  CONSTRAINT chk_email_length CHECK (length("email") >= 3 AND length("email") <= 254),
  CONSTRAINT chk_password_length CHECK (length("password") <= 1024)
);
