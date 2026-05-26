CREATE TABLE "accounts_members" (
  "id"         SERIAL NOT NULL,
  "account_id" INTEGER NOT NULL,
  "group_id"   INTEGER NOT NULL,
  "created"    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY ("id"),
  FOREIGN KEY ("account_id")
    REFERENCES "accounts_accounts" ("id"),
  FOREIGN KEY ("group_id")
    REFERENCES "accounts_groups" ("id")
);

