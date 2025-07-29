DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'unique_feeds_link') THEN
        ALTER TABLE feeds ADD CONSTRAINT unique_feeds_link UNIQUE (link);
    END IF;
END
$$; 