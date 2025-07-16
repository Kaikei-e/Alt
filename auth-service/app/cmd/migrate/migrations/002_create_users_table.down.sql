-- Drop users table and related objects
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
DROP TABLE IF EXISTS users CASCADE;