//go:build integration

// Package pgtest runs driver tests against a real PostgreSQL carrying the
// real Atlas migration history.
//
// The driver packages are otherwise covered by pgxmock, which replays a
// recorded script: it proves a query was issued with the arguments the caller
// meant, and nothing about what the server does with it. Locking, constraint
// enforcement, transaction visibility and the migrated schema itself are all
// invisible to it, so the tests that need those live here instead.
//
// One container serves the whole test binary. Migrations are applied once
// into a template database, and every NewDB call clones that template, which
// PostgreSQL does as a file copy rather than by replaying 100+ migrations.
// Tests therefore start from an identical schema without sharing rows, and
// may run in parallel.
package pgtest

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Both pinned to what compose/compose.staging.yaml and
	// migrations-atlas/docker/Dockerfile run, so a schema that applies here
	// is the schema the deployed stack gets. Bump them together.
	postgresImage = "postgres:17-alpine"
	atlasImage    = "arigaio/atlas:1.2.0-alpine"

	// The migration history contains `ALTER TABLE ... OWNER TO alt_db_user`,
	// so the role has to exist before Atlas runs. Deployed stacks get it for
	// free by creating the cluster under that name; the harness matches them
	// rather than patching the history for the benefit of tests.
	adminUser  = "alt_db_user"
	templateDB = "alt"

	// adminDB is the maintenance database every cluster ships with. CREATE
	// DATABASE cannot run inside a transaction and cannot target the database
	// the session is connected to, so cloning the template needs a third one.
	adminDB = "postgres"

	// Never leaves the container: the port is published on an ephemeral
	// loopback binding that lives and dies with the test binary.
	adminPass = "pgtest"

	startTimeout = 3 * time.Minute
)

var (
	once     sync.Once
	shared   *harness
	startErr error
)

type harness struct {
	// adminDSN points at the maintenance database. CREATE DATABASE cannot run
	// inside a transaction and cannot target the database you are connected
	// to, so cloning the template needs a session outside both.
	adminDSN string
}

// NewDB returns a pool bound to a fresh database cloned from the migrated
// template. The database is dropped when the test ends.
func NewDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	once.Do(func() { shared, startErr = start() })
	if startErr != nil {
		t.Fatalf("pgtest: start postgres: %v", startErr)
	}

	ctx := context.Background()

	admin, err := pgxpool.New(ctx, shared.adminDSN)
	if err != nil {
		t.Fatalf("pgtest: connect admin database: %v", err)
	}
	defer admin.Close()

	// A UUID rather than the test name: subtests carry '/' and case names are
	// free text, neither of which survives as an identifier.
	name := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, templateDB)); err != nil {
		t.Fatalf("pgtest: clone template into %s: %v", name, err)
	}

	cfg, err := pgxpool.ParseConfig(dsn(shared.adminDSN, name))
	if err != nil {
		t.Fatalf("pgtest: parse dsn for %s: %v", name, err)
	}
	// The deployed pools run simple protocol for PgBouncer transaction pooling
	// (alt_db/init.go), and the protocol decides how arguments are encoded:
	// under simple protocol pgx interpolates them client-side, so a []byte
	// bound to a JSONB column becomes a bytea hex literal rather than JSON.
	// A harness left on the extended-protocol default accepts statements the
	// deployed stack rejects, which is how the push fan-out reached production
	// failing every insert with a green suite behind it.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgtest: connect %s: %v", name, err)
	}

	t.Cleanup(func() {
		pool.Close()

		drop, err := pgxpool.New(context.Background(), shared.adminDSN)
		if err != nil {
			t.Logf("pgtest: reconnect to drop %s: %v", name, err)
			return
		}
		defer drop.Close()

		// WITH (FORCE) terminates sessions the test leaked. Without it a
		// forgotten connection turns cleanup into a hang, and the failure
		// surfaces in whichever test happens to run next.
		if _, err := drop.Exec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", name)); err != nil {
			t.Logf("pgtest: drop %s: %v", name, err)
		}
	})

	return pool
}

func start() (*harness, error) {
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	net, err := network.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create network: %w", err)
	}

	// The migration history GRANTs to roles it never creates
	// (tag_generator, search_indexer_user, ...). Deployed stacks satisfy
	// that from the entrypoint's init directory, and the staging fixture is
	// the same file, so the harness reuses it rather than keeping a second
	// list that would drift the first time a GRANT is added.
	roles, err := repoPath("e2e", "fixtures", "alt-backend", "db-init", "01-create-roles.sql")
	if err != nil {
		return nil, err
	}

	pg, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase(templateDB),
		postgres.WithUsername(adminUser),
		postgres.WithPassword(adminPass),
		postgres.WithInitScripts(roles),
		network.WithNetwork([]string{"postgres"}, net),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp"),
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("run postgres: %w", err)
	}

	if err := migrate(ctx, net.Name); err != nil {
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	host, err := pg.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve host: %w", err)
	}
	port, err := pg.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, fmt.Errorf("resolve mapped port: %w", err)
	}

	return &harness{
		adminDSN: fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			adminUser, adminPass, host, port.Port(), adminDB),
	}, nil
}

// migrate runs the same Atlas binary and the same migration directory the
// deployed migrator image carries, against the template database over the
// container network. Applying the committed history rather than a
// hand-maintained schema dump is the point: a migration that only works
// against an already-populated database fails here too.
func migrate(ctx context.Context, networkName string) error {
	dir, err := repoPath("migrations-atlas", "migrations")
	if err != nil {
		return err
	}

	url := fmt.Sprintf("postgres://%s:%s@postgres:5432/%s?sslmode=disable&search_path=public",
		adminUser, adminPass, templateDB)

	req := testcontainers.ContainerRequest{
		Image:    atlasImage,
		Networks: []string{networkName},
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.Binds = []string{dir + ":/migrations:ro"}
		},
		Cmd: []string{
			"migrate", "apply",
			"--url", url,
			"--dir", "file:///migrations",
			"--revisions-schema", "public",
		},
		WaitingFor: wait.ForExit().WithExitTimeout(startTimeout),
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("run atlas: %w", err)
	}
	defer func() { _ = c.Terminate(context.Background()) }()

	state, err := c.State(ctx)
	if err != nil {
		return fmt.Errorf("read atlas exit state: %w", err)
	}
	if state.ExitCode != 0 {
		logs, _ := c.Logs(ctx)
		out := ""
		if logs != nil {
			defer func() { _ = logs.Close() }()
			b, _ := io.ReadAll(logs)
			out = string(b)
		}
		return fmt.Errorf("atlas migrate apply exited %d:\n%s", state.ExitCode, out)
	}

	return nil
}

// repoPath resolves a repository-relative path from this source file, so the
// harness works from whatever working directory `go test` was invoked in.
func repoPath(parts ...string) (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve caller for %s", filepath.Join(parts...))
	}

	// .../alt-backend/app/test_utils/pgtest/pgtest.go -> repository root
	root := filepath.Join(filepath.Dir(self), "..", "..", "..", "..")
	path, err := filepath.Abs(filepath.Join(append([]string{root}, parts...)...))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", filepath.Join(parts...), err)
	}
	return path, nil
}

func dsn(adminDSN, database string) string {
	i := strings.LastIndex(adminDSN, "/"+adminDB)
	return adminDSN[:i] + "/" + database + adminDSN[i+len("/"+adminDB):]
}
