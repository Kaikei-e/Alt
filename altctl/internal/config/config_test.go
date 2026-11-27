package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alt-project/altctl/internal/fsperm"
	"github.com/stretchr/testify/require"
)

func TestLoad_MissingConfigFileFallsBackToDefaults(t *testing.T) {
	emptyDir := t.TempDir()
	t.Chdir(emptyDir)
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load("", emptyDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	require.Equal(t, emptyDir, cfg.Project.Root)
	require.Equal(t, "default", cfg.Project.DockerContext)
	require.Equal(t, "compose", cfg.Compose.Dir)
	require.Equal(t, "base.yaml", cfg.Compose.BaseFile)
	require.Equal(t, []string{"db", "auth", "core", "workers"}, cfg.Defaults.Stacks)
	require.Equal(t, "info", cfg.Logging.Level)
	require.Equal(t, "text", cfg.Logging.Format)
	require.True(t, cfg.Output.Colors)
	require.True(t, cfg.Output.Progress)
	require.Equal(t, filepath.Join(emptyDir, ".altctl.yaml"), cfg.ConfigFilePath)
}

func TestLoad_MalformedYAMLReturnsError(t *testing.T) {
	const malformedYAML = "logging:\n  level: [unclosed\n"

	t.Run("discovered .altctl.yaml", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		t.Setenv("HOME", t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".altctl.yaml"), []byte(malformedYAML), fsperm.SecretFile))

		cfg, err := Load("", dir)
		require.ErrorContains(t, err, "reading config")
		require.Nil(t, cfg)
	})

	t.Run("explicit cfgFile", func(t *testing.T) {
		dir := t.TempDir()
		badPath := filepath.Join(dir, "broken.yaml")
		require.NoError(t, os.WriteFile(badPath, []byte(malformedYAML), fsperm.SecretFile))

		cfg, err := Load(badPath, dir)
		require.ErrorContains(t, err, "reading config")
		require.Nil(t, cfg)
	})
}

func TestLoad_ExplicitNonexistentConfigFileReturnsError(t *testing.T) {
	projectDir := t.TempDir()
	missingPath := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	cfg, err := Load(missingPath, projectDir)
	require.ErrorContains(t, err, "reading config")
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Nil(t, cfg)
}

func TestLoad_ValidConfigOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())

	cfgPath := filepath.Join(dir, ".altctl.yaml")
	body := "compose:\n  dir: custom-compose\nlogging:\n  level: debug\n  format: json\ndefaults:\n  stacks:\n    - db\n    - core\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(body), fsperm.SecretFile))

	cfg, err := Load("", dir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	require.Equal(t, "custom-compose", cfg.Compose.Dir)
	require.Equal(t, "debug", cfg.Logging.Level)
	require.Equal(t, "json", cfg.Logging.Format)
	require.Equal(t, []string{"db", "core"}, cfg.Defaults.Stacks)
	require.Equal(t, cfgPath, cfg.ConfigFilePath)
}
