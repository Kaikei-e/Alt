package knowledge_loop_projector

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Static, DB-less invariant guards for the Knowledge Loop canonical contract.
// These tests parse migration SQL on disk and refuse to compile contracts that
// would silently re-introduce forbidden columns. They run as part of `go test`
// without a database connection so the breakage surfaces immediately in CI.
//
// Canonical contract (docs/plan/knowledge-loop-canonical-contract.md):
//   §3 invariant 2 — `updated_at` is forbidden on Knowledge Loop projection tables
//                    (use projection_seq_hiwater / projection_revision /
//                     freshness_at / source_observed_at instead).
//   §10           — `projected_at` is debug-only; it MUST NOT be exposed via
//                    API, proto, public view, metrics, or production logs.
//   §16 acceptance — items 2, 12.
//
// When you intentionally bend these rules (almost certainly a mistake), update
// the contract first, then this test, then the migration. Never the other way
// round.

const migrationsRelDir = "../../../migrations"

func loadMigration(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(migrationsRelDir, name)
	body, err := os.ReadFile(path)
	require.NoErrorf(t, err, "open migration %s", path)
	return string(body)
}

// extractCreateTable returns the body of `CREATE TABLE <name> ( ... );` from
// the migration text. Whitespace and case are normalised. Returns "" if the
// table is not declared in this migration.
func extractCreateTable(sql, table string) string {
	re := regexp.MustCompile(`(?is)CREATE\s+TABLE\s+` + regexp.QuoteMeta(table) + `\s*\(([^;]+)\)\s*;`)
	m := re.FindStringSubmatch(sql)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// extractCreateView returns the body of `CREATE VIEW <name> AS SELECT ... FROM ...;`
// from the migration text. Returns "" if the view is not declared here.
func extractCreateView(sql, view string) string {
	re := regexp.MustCompile(`(?is)CREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+` + regexp.QuoteMeta(view) + `\s+AS\s*([^;]+);`)
	m := re.FindStringSubmatch(sql)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// columnList parses a CREATE TABLE body and returns the bare column names
// (skipping CONSTRAINT / CHECK / PRIMARY KEY rows). Best-effort: this is a
// linter, not a SQL parser, so only call it on tables we author.
func columnList(body string) []string {
	cols := []string{}
	for raw := range strings.SplitSeq(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		// Strip trailing comma.
		line = strings.TrimRight(line, ",")
		// Skip table-level constraints.
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "CONSTRAINT ") ||
			strings.HasPrefix(upper, "CHECK ") ||
			strings.HasPrefix(upper, "PRIMARY KEY") ||
			strings.HasPrefix(upper, "FOREIGN KEY") ||
			strings.HasPrefix(upper, "UNIQUE ") {
			continue
		}
		// First whitespace-delimited token is the column name.
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		cols = append(cols, fields[0])
	}
	return cols
}

func TestKnowledgeLoopEntries_NoUpdatedAtColumn(t *testing.T) {
	t.Parallel()
	sql := loadMigration(t, "00011_create_knowledge_loop_tables.sql")

	tables := []string{
		"knowledge_loop_entries",
		"knowledge_loop_session_state",
		"knowledge_loop_surfaces",
		"knowledge_loop_transition_dedupes",
	}
	for _, tbl := range tables {
		body := extractCreateTable(sql, tbl)
		require.NotEmptyf(t, body, "table %s not found in migration", tbl)
		cols := columnList(body)
		for _, c := range cols {
			require.NotEqualf(t, "updated_at", strings.ToLower(c),
				"%s.updated_at is forbidden by canonical contract §3 invariant 2", tbl)
		}
	}
}

func TestKnowledgeLoopEntriesPublic_DoesNotExposeProjectedAt(t *testing.T) {
	t.Parallel()
	sql := loadMigration(t, "00011_create_knowledge_loop_tables.sql")
	body := extractCreateView(sql, "knowledge_loop_entries_public")
	require.NotEmpty(t, body, "knowledge_loop_entries_public not found")
	require.NotContainsf(t, strings.ToLower(body), "projected_at",
		"knowledge_loop_entries_public MUST NOT SELECT projected_at — canonical contract §10/§16 item 12")

	// The base table still keeps `projected_at` for operational debug; that is
	// fine. Just make sure it's there so we know the view is genuinely
	// excluding it (not that we forgot the column entirely).
	tableBody := extractCreateTable(sql, "knowledge_loop_entries")
	require.Contains(t, strings.ToLower(tableBody), "projected_at",
		"projected_at debug column missing from knowledge_loop_entries — the view exclusion is meaningless without the column existing")
}

// TestProjectedAtNotInAnyPublicView guards future migrations that might add
// new public views. Each refresh of `knowledge_loop_entries_public` must keep
// the exclusion. Scans every migration file for any CREATE VIEW that mentions
// `knowledge_loop` in its name.
func TestProjectedAtNotInAnyPublicView(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(migrationsRelDir)
	require.NoError(t, err)

	viewRe := regexp.MustCompile(`(?is)CREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+(knowledge_loop[A-Za-z0-9_]+)\s+AS\s*([^;]+);`)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(migrationsRelDir, e.Name()))
		require.NoError(t, err)
		for _, m := range viewRe.FindAllStringSubmatch(string(body), -1) {
			viewName := m[1]
			viewBody := strings.ToLower(m[2])
			require.NotContainsf(t, viewBody, "projected_at",
				"%s in %s exposes projected_at — canonical contract §10 forbids this on any Loop public view",
				viewName, e.Name())
		}
	}
}

// TestKnowledgeLoopEntries_ForbiddenColumnNames is the regression guard for
// the broader class of "wall-clock ↔ business-fact confusion" mistakes. Any
// of these names being added to a Loop projection table is almost certainly
// a hidden-event smell that the canonical contract decomposed away.
func TestKnowledgeLoopEntries_ForbiddenColumnNames(t *testing.T) {
	t.Parallel()
	sql := loadMigration(t, "00011_create_knowledge_loop_tables.sql")

	forbidden := []string{
		"updated_at",
		"modified_at",
		"last_updated_at",
		"last_modified_at",
	}

	tables := []string{
		"knowledge_loop_entries",
		"knowledge_loop_session_state",
		"knowledge_loop_surfaces",
	}
	for _, tbl := range tables {
		body := strings.ToLower(extractCreateTable(sql, tbl))
		require.NotEmptyf(t, body, "table %s not found", tbl)
		for _, f := range forbidden {
			require.NotContainsf(t, body, " "+f+" ",
				"%s contains forbidden column %q (canonical contract §3 invariant 2 — use projection_seq_hiwater / projection_revision / freshness_at instead)",
				tbl, f)
		}
	}
}
