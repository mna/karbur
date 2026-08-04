CREATE TABLE "tokens_tokens" (
	"token"         TEXT NOT NULL,
	"type"          TEXT NOT NULL,
	"single_use"    BOOLEAN NOT NULL,
	"ref_id"        UUID NOT NULL,
	"expiry"        TIMESTAMPTZ NOT NULL,
	"idle"          TIMESTAMPTZ NULL,
	"idle_duration" INTEGER NULL,
	-- not using jsonb because write speed is important and json querying is not.
  "data"          JSON NOT NULL DEFAULT 'null',
	"created"       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

	PRIMARY KEY ("token"),
	CONSTRAINT chk_token_length CHECK (length("token") >= 10 AND length("token") <= 100),
	CONSTRAINT chk_type_length CHECK (length("type") > 0 AND length("type") <= 100),
	CONSTRAINT chk_idle_multi_use_only CHECK ("idle" IS NULL OR "single_use" IS FALSE)
);
