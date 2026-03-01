-- Add og_image_url column to feeds table.
-- Populated during hourly feed collection from RSS Item.Image / Enclosures / media extensions.
-- No additional HTTP request needed — extracted from RSS XML.
ALTER TABLE feeds ADD COLUMN IF NOT EXISTS og_image_url TEXT;
