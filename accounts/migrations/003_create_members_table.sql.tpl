CREATE TABLE "accounts_members" (
  "id"         SERIAL NOT NULL,
  "account_id" INTEGER NOT NULL,
  "group_id"   INTEGER NOT NULL,
  "created"    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY ("id"),
  CONSTRAINT uidx_members_account_id_group_id UNIQUE ("account_id", "group_id"),
  CONSTRAINT fk_members_account_id FOREIGN KEY ("account_id")
    REFERENCES "accounts_accounts" ("id") ON DELETE CASCADE,
  CONSTRAINT fk_members_group_id FOREIGN KEY ("group_id")
    REFERENCES "accounts_groups" ("id") ON DELETE CASCADE
);

