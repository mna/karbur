CREATE TABLE "tokens_tokens" (
	"token"      TEXT NOT NULL CHECK (length("token") >= 10 AND length("token") <= 100),
	"type"       TEXT NOT NULL CHECK (length("type") > 0 AND length("type") <= 100),
	"single_use" BOOLEAN NOT NULL,
	"ref_id"     BIGINT NOT NULL,
	"expiry"     TIMESTAMPTZ NOT NULL,
	"created"    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

	PRIMARY KEY ("token")
);
