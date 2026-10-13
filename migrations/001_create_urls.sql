CREATE TABLE urls (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    url TEXT NOT NULL UNIQUE CHECK (octet_length(url) <= 2048)
);