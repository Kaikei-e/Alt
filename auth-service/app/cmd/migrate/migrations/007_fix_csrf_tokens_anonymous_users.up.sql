-- Fix csrf_tokens table to support anonymous users (X27 Browser Flow)
-- Allow user_id to be NULL for anonymous CSRF tokens

-- Drop the existing foreign key constraint
ALTER TABLE csrf_tokens DROP CONSTRAINT IF EXISTS csrf_tokens_user_id_fkey;

-- Alter user_id column to allow NULL values
ALTER TABLE csrf_tokens ALTER COLUMN user_id DROP NOT NULL;

-- Add new foreign key constraint that allows NULL values
ALTER TABLE csrf_tokens 
ADD CONSTRAINT csrf_tokens_user_id_fkey 
FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Create index for anonymous sessions (NULL user_id)
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_anonymous ON csrf_tokens(session_id, expires_at) WHERE user_id IS NULL;

-- Create index for authenticated users  
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_authenticated ON csrf_tokens(user_id, session_id, expires_at) WHERE user_id IS NOT NULL;

-- Add comment for clarity
COMMENT ON COLUMN csrf_tokens.user_id IS 'NULL for anonymous sessions, UUID for authenticated users';