ALTER TABLE short_urls
ADD COLUMN revocation_token_hash BYTEA
CHECK(octet_length(revocation_token_hash) = 32)