package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlags_RequiresDSN(t *testing.T) {
	t.Setenv("ARTICLES_DB_DSN", "")
	t.Setenv("DATABASE_URL", "")
	_, err := parseFlags([]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DSN is required")
}

func TestParseFlags_FlagBeatsEnv(t *testing.T) {
	t.Setenv("ARTICLES_DB_DSN", "postgres://env:pass@h:5432/db")
	opts, err := parseFlags([]string{"--dsn", "postgres://flag:pass@h:5432/db"})
	require.NoError(t, err)
	assert.Equal(t, "postgres://flag:pass@h:5432/db", opts.DSN)
}

func TestParseFlags_EnvFallbackOrder(t *testing.T) {
	t.Setenv("ARTICLES_DB_DSN", "")
	t.Setenv("DATABASE_URL", "postgres://db-url:pass@h:5432/db")
	opts, err := parseFlags([]string{})
	require.NoError(t, err)
	assert.Equal(t, "postgres://db-url:pass@h:5432/db", opts.DSN)
}

func TestParseFlags_RejectsBadBatchSize(t *testing.T) {
	t.Setenv("ARTICLES_DB_DSN", "postgres://x")
	_, err := parseFlags([]string{"--batch-size", "0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--batch-size")
}

func TestParseFlags_RejectsNegativeThrottle(t *testing.T) {
	t.Setenv("ARTICLES_DB_DSN", "postgres://x")
	_, err := parseFlags([]string{"--throttle-ms", "-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--throttle-ms")
}

func TestParseFlags_Defaults(t *testing.T) {
	t.Setenv("ARTICLES_DB_DSN", "postgres://x")
	opts, err := parseFlags([]string{})
	require.NoError(t, err)
	assert.Equal(t, 500, opts.BatchSize)
	assert.Equal(t, 100, opts.ThrottleMs)
	assert.False(t, opts.DryRun)
	assert.Empty(t, opts.ResumeFrom)
}

func TestMain_ParseFlagsHelpExitsZero(t *testing.T) {
	// Ensure --help produces flag.ErrHelp path rather than a parse error.
	t.Setenv("ARTICLES_DB_DSN", "postgres://x")
	// parseFlags returns flag.ErrHelp when --help is passed; main.go maps it
	// to exit code 0. We only verify the parsing side effect here.
	_, err := parseFlags([]string{"--help"})
	require.Error(t, err)
	// flag.ContinueOnError returns flag.ErrHelp
	assert.Equal(t, "flag: help requested", err.Error())

	// Discard flag package's help output so test output stays clean.
	_ = os.Stdout
}
