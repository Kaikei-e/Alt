-- Drop service users that no longer need direct DB access (ADR-000241 completion)
-- These services now use Connect-RPC via alt-backend exclusively.
-- REASSIGN OWNED must run before DROP ROLE to transfer any object ownership.
DO $$
DECLARE
  db_owner text;
BEGIN
  -- Determine the database owner to reassign objects to
  SELECT pg_catalog.pg_get_userbyid(d.datdba) INTO db_owner
  FROM pg_catalog.pg_database d
  WHERE d.datname = current_database();

  -- tag_generator
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tag_generator') THEN
    EXECUTE format('REASSIGN OWNED BY tag_generator TO %I', db_owner);
    EXECUTE 'DROP OWNED BY tag_generator';
    DROP ROLE tag_generator;
  END IF;

  -- search_indexer_user
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'search_indexer_user') THEN
    EXECUTE format('REASSIGN OWNED BY search_indexer_user TO %I', db_owner);
    EXECUTE 'DROP OWNED BY search_indexer_user';
    DROP ROLE search_indexer_user;
  END IF;

  -- pre_processor_user
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pre_processor_user') THEN
    EXECUTE format('REASSIGN OWNED BY pre_processor_user TO %I', db_owner);
    EXECUTE 'DROP OWNED BY pre_processor_user';
    DROP ROLE pre_processor_user;
  END IF;
END;
$$;
