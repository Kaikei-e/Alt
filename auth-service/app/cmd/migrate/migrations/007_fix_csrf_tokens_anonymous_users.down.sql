-- Rollback: Revert csrf_tokens table to require user_id (not recommended in production)

-- Drop the indexes we created
DROP INDEX IF EXISTS idx_csrf_tokens_anonymous;
DROP INDEX IF EXISTS idx_csrf_tokens_authenticated;

-- Remove comment
COMMENT ON COLUMN csrf_tokens.user_id IS NULL;

-- Drop foreign key constraint
ALTER TABLE csrf_tokens DROP CONSTRAINT IF EXISTS csrf_tokens_user_id_fkey;

-- Delete all anonymous CSRF tokens (they will become invalid)
DELETE FROM csrf_tokens WHERE user_id IS NULL;

-- Restore NOT NULL constraint
ALTER TABLE csrf_tokens ALTER COLUMN user_id SET NOT NULL;

-- Restore original foreign key constraint
ALTER TABLE csrf_tokens 
ADD CONSTRAINT csrf_tokens_user_id_fkey 
FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;