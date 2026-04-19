-- alt-backend staging DB init — creates every role referenced by a GRANT
-- in migrations-atlas/migrations/*.sql as NOLOGIN. Staging has no
-- other services connecting, so LOGIN + password aren't required; we
-- only need the roles to exist so GRANT doesn't fail.
--
-- The production counterpart is db/init/01-create-users.sh which also
-- sets LOGIN passwords from Docker secrets. Keeping staging minimal
-- avoids the need to mount secrets files into the ephemeral Postgres.

DO $$
DECLARE
    role_name text;
    role_list text[] := ARRAY[
        'alt_appuser',
        'alt_db_user',
        'db_owner',
        'pre_processor_user',
        'pre_processor_sidecar_user',
        'search_indexer_user',
        'tag_generator'
    ];
BEGIN
    FOREACH role_name IN ARRAY role_list LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('CREATE ROLE %I', role_name);
        END IF;
    END LOOP;
END;
$$;
