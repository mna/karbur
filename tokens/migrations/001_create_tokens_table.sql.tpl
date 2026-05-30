CREATE TABLE "tokens_tokens" (
	"token"      TEXT NOT NULL,
	"type"       TEXT NOT NULL,
	"single_use" BOOLEAN NOT NULL,
	"ref_id"     BIGINT NOT NULL,
	"expiry"     TIMESTAMPTZ NOT NULL,
	"created"    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

	PRIMARY KEY ("token"),
	CONSTRAINT chk_token_length CHECK (length("token") >= 10 AND length("token") <= 100),
	CONSTRAINT chk_type_length CHECK (length("type") > 0 AND length("type") <= 100)
);
