-- this is a 1:1 relationship with tokens_tokens, but creating a distinct
-- table as this is unrelated to the tokens package, only applies to the
-- accounts.
CREATE TABLE "accounts_session_data" (
	"token"   TEXT NOT NULL,
	-- not using jsonb because write speed is important and json querying is not.
  "data"    JSON NOT NULL DEFAULT '{}',
  "created" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY ("token"),
  CONSTRAINT fk_tokens_token FOREIGN KEY ("token")
    REFERENCES "tokens_tokens" ("token") ON UPDATE CASCADE ON DELETE CASCADE
);

