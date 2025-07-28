-- Revoke the privileges granted to tag_generator in migration 000031

REVOKE SELECT, INSERT ON article_tags FROM tag_generator;
REVOKE SELECT, INSERT ON tags FROM tag_generator;
REVOKE SELECT ON articles FROM tag_generator;
REVOKE USAGE ON SCHEMA public FROM tag_generator;
REVOKE CONNECT ON DATABASE alt FROM tag_generator;