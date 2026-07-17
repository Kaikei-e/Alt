package sovereign_db

// activeProjectionVersionSQL is the shared subquery that resolves the currently
// active projection version (falling back to 1 when none is marked active).
const activeProjectionVersionSQL = `COALESCE((
	SELECT version FROM knowledge_projection_versions
	WHERE status = 'active'
	ORDER BY version DESC LIMIT 1
), 1)`
