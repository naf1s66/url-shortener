CREATE TABLE urls (
    -- PostgreSQL generates a UUID for each URL record using gen_random_uuid().
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    -- UNIQUE checks against duplication; octet_length check enforces the 2048 byte limit.
    url TEXT NOT NULL UNIQUE CHECK (octet_length(url) <= 2048)
);