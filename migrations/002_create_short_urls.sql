CREATE TABLE short_urls (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    short_code VARCHAR(6) NOT NULL UNIQUE CHECK (short_code ~ '^[A-Za-z0-9]{6}$'),
    url_id UUID NOT NULL REFERENCES urls(id),
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);