-- auth-hub staging DB init.
--
-- Kratos manages its own schema and roles on kratos-db. The staging slice
-- runs a single ephemeral Postgres owned entirely by Kratos, so unlike
-- alt-backend (see e2e/fixtures/alt-backend/db-init/01-create-roles.sql)
-- there are no cross-service roles to pre-create. This file keeps the
-- /docker-entrypoint-initdb.d hook present for ADR-000781 parity: future
-- seed SQL for Kratos admin data can land here without compose edits.

SELECT 1;
