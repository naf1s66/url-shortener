CREATE TABLE short_urls (
    -- Short url identifier
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    -- UNIQUE to prevent duplicate codes, including during concurrent requests.
    -- The regex enforces database layer six alphanumeric characters validation.
    -- UNIQUE also generates B-tree index
    -- Reducing lookup time complexity from O(N) to O(log N).
    short_code VARCHAR(6) NOT NULL UNIQUE CHECK (short_code ~ '^[A-Za-z0-9]{6}$'),
    -- No ON DELETE CASCADE to satisfy system design requirements.
    -- For security, codes will be reserved forever and never be reused
    -- Even after revocation or expiration.
    url_id UUID NOT NULL REFERENCES urls(id),
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);