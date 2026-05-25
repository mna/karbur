CREATE PROCEDURE "tokens_cleanup" ()
LANGUAGE SQL
AS $$
  DELETE FROM
    "tokens_tokens"
  WHERE
    "expiry" < now()
$$;

