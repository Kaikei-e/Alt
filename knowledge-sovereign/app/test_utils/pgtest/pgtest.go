//go:build integration

// Package pgtest runs knowledge-sovereign driver tests against a real
// PostgreSQL carrying the real Atlas migration history.
//
// The driver package is otherwise covered by hand-written pgx fakes, which
// replay a recorded script: they prove a query was issued with the arguments
// the caller meant, and nothing about what the server does with it. Whether the
// named tables exist, whether TRUNCATE is accepted over that table set, whether
// a constraint or a foreign key would reject a statement, what another session
// sees mid-transaction, and whether a rollback really restores the rows are all
// invisible to a fake. The tests that need those live behind this harness.
//
// One container serves the whole test binary. Migrations are applied once into
// a template database, and every NewDB call clones that template, which
// PostgreSQL does as a file copy rather than by replaying the whole history.
// Tests therefore start from an identical schema without sharing rows, and may
// run in parallel.
//
// Run them with:
//
//	cd knowledge-sovereign/app && go test -tags integration ./driver/... ./test_utils/...
//
// (knowledge-sovereign has no Makefile; alt-backend's sibling harness is driven
// by `make -f Makefile.local test-integration`.)
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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// postgresImage is what compose/sovereign.yaml runs for
	// knowledge-sovereign-db, and atlasImage is the base of
	// knowledge-sovereign/docker/Dockerfile.migrator. Neither follows
	// alt-backend's pins — that stack is on postgres 17 — so a schema that
	// applies here is the schema the deployed sovereign stack gets. Bump them
	// together with those two files.
	postgresImage = "postgres:16-alpine"
	atlasImage    = "arigaio/atlas:1.2.0-alpine"

	// The database name and role the deployed cluster is created under. The
	// migration history contains no GRANT and no ALTER ... OWNER TO, so unlike
	// alt-backend's harness this one needs no role-creation init script; if a
	// migration ever adds one, the fixture belongs next to the compose stack,
	// not here.
	adminUser  = "sovereign"
	templateDB = "knowledge_sovereign"

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
	// adminDSN points at the maintenance database, outside both the template
	// and the clone.
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

	pool, err := pgxpool.New(ctx, dsn(shared.adminDSN, name))
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

	pg, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase(templateDB),
		postgres.WithUsername(adminUser),
		postgres.WithPassword(adminPass),
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
// hand-maintained schema dump is the point: knowledge-sovereign's history
// rewrites knowledge_events into a partitioned table and copies the rows across
// (00006), so a migration that only works against a particular starting state
// fails here too.
func migrate(ctx context.Context, networkName string) error {
	dir, err := servicePath("migrations")
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

// servicePath resolves a knowledge-sovereign-relative path from this source
// file, so the harness works from whatever working directory `go test` was
// invoked in.
func servicePath(parts ...string) (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve caller for %s", filepath.Join(parts...))
	}

	// .../knowledge-sovereign/app/test_utils/pgtest/pgtest.go -> service root
	root := filepath.Join(filepath.Dir(self), "..", "..", "..")
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
