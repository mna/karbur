CREATE UNIQUE INDEX uidx_tokens_type_ref_id ON "tokens_tokens" ("type", "ref_id") WHERE "single_use";
