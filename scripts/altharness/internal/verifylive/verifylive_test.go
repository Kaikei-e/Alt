package verifylive_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/verifylive"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if st, statErr := os.Stat(filepath.Join(dir, "scripts", "altharness", "go.mod")); statErr == nil && !st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root from %s", wd)
		}
		dir = parent
	}
}

func makeSyntheticRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	verifyDir := filepath.Join(root, ".claude", "skills", "verify-alt")
	featuresDir := filepath.Join(verifyDir, "features")
	maintainDir := filepath.Join(root, ".claude", "skills", "maintain-verification-skill")
	agentSkillsDir := filepath.Join(root, ".agents", "skills")
	for _, d := range []string{featuresDir, maintainDir, agentSkillsDir} {
		if err := os.MkdirAll(d, fsperm.Dir); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(verifyDir, "SKILL.md"), []byte("# verify-alt\n"), fsperm.File); err != nil {
		t.Fatalf("write verify SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(maintainDir, "SKILL.md"), []byte("# maintain\n"), fsperm.File); err != nil {
		t.Fatalf("write maintain SKILL.md: %v", err)
	}
	if err := os.Symlink("../../.claude/skills/verify-alt", filepath.Join(agentSkillsDir, "verify-alt")); err != nil {
		t.Fatalf("symlink verify-alt: %v", err)
	}
	if err := os.Symlink("../../.claude/skills/maintain-verification-skill", filepath.Join(agentSkillsDir, "maintain-verification-skill")); err != nil {
		t.Fatalf("symlink maintain-verification-skill: %v", err)
	}
	readme := "# Feature Map\n\n| Feature | Doc |\n|---|---|\n| Sample | [sample.md](sample.md) |\n"
	if err := os.WriteFile(filepath.Join(featuresDir, "README.md"), []byte(readme), fsperm.File); err != nil {
		t.Fatalf("write features README: %v", err)
	}
	featureDoc := "# Sample\n\n## Sub-features\n- a\n\n## How to get to it\n- b\n\n## Driving it with the harness\n- c\n\n## Gotchas\n- d\n"
	if err := os.WriteFile(filepath.Join(featuresDir, "sample.md"), []byte(featureDoc), fsperm.File); err != nil {
		t.Fatalf("write sample.md: %v", err)
	}
	// Doctor requires an interpreter that can import the SSE handler; the
	// synthetic repo has no news-creator, so a stub stands in for its .venv.
	venvBin := filepath.Join(root, "news-creator", "app", ".venv", "bin")
	if err := os.MkdirAll(venvBin, fsperm.Dir); err != nil {
		t.Fatalf("mkdir %s: %v", venvBin, err)
	}
	if err := fsperm.WriteExecutable(filepath.Join(venvBin, "python"), []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatalf("write stub python: %v", err)
	}
	return root
}

func TestDoctor_RejectsProductionComposeProject(t *testing.T) {
	t.Parallel()
	root := makeSyntheticRepo(t)

	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		wantCode int
	}{
		{
			name:     "env_alt_rejected",
			args:     []string{"doctor", "--repo-root", root, "--format", "json"},
			env:      map[string]string{"COMPOSE_PROJECT_NAME": "alt"},
			wantCode: 1,
		},
		{
			name:     "env_ALT_uppercase_rejected",
			args:     []string{"doctor", "--repo-root", root, "--format", "json"},
			env:      map[string]string{"COMPOSE_PROJECT_NAME": " ALT "},
			wantCode: 1,
		},
		{
			name:     "flag_project_alt_rejected",
			args:     []string{"doctor", "--repo-root", root, "--project", "alt", "--format", "json"},
			env:      map[string]string{"COMPOSE_PROJECT_NAME": "alt-verify-1"},
			wantCode: 1,
		},
		{
			name:     "non_prod_project_allowed",
			args:     []string{"doctor", "--repo-root", root, "--project", "alt-verify-1", "--format", "json"},
			env:      map[string]string{"COMPOSE_PROJECT_NAME": "alt-verify-1"},
			wantCode: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			code := verifylive.RunWithOptions(context.Background(), tc.args, verifylive.Options{
				Stdout: &out,
				Stderr: &errBuf,
				Env:    tc.env,
			})
			if code != tc.wantCode {
				t.Fatalf("got code %d, want %d\nstdout=%s\nstderr=%s", code, tc.wantCode, out.String(), errBuf.String())
			}
			var report verifylive.DoctorReport
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatalf("unmarshal doctor report: %v\nraw=%s", err, out.String())
			}
			if tc.wantCode == 0 && !report.OK {
				t.Fatalf("expected report.OK=true, got false: %+v", report)
			}
			if tc.wantCode != 0 && report.OK {
				t.Fatalf("expected report.OK=false, got true: %+v", report)
			}
		})
	}
}

func TestDoctor_FeatureMapHygiene(t *testing.T) {
	t.Parallel()

	t.Run("valid_synthetic_repo", func(t *testing.T) {
		t.Parallel()
		root := makeSyntheticRepo(t)
		var out, errBuf bytes.Buffer
		code := verifylive.RunWithOptions(context.Background(), []string{"doctor", "--repo-root", root, "--format", "json"}, verifylive.Options{
			Stdout: &out,
			Stderr: &errBuf,
			Env:    map[string]string{"COMPOSE_PROJECT_NAME": ""},
		})
		if code != 0 {
			t.Fatalf("expected doctor pass, got %d\nstdout=%s\nstderr=%s", code, out.String(), errBuf.String())
		}
	})

	t.Run("catches_orphan_feature_file", func(t *testing.T) {
		t.Parallel()
		root := makeSyntheticRepo(t)
		orphan := filepath.Join(root, ".claude", "skills", "verify-alt", "features", "unlisted.md")
		doc := "# Unlisted\n\n## Sub-features\n\n## How to get to it\n\n## Driving it with the harness\n\n## Gotchas\n"
		if err := os.WriteFile(orphan, []byte(doc), fsperm.File); err != nil {
			t.Fatalf("write orphan: %v", err)
		}
		var out, errBuf bytes.Buffer
		code := verifylive.RunWithOptions(context.Background(), []string{"doctor", "--repo-root", root, "--format", "json"}, verifylive.Options{
			Stdout: &out,
			Stderr: &errBuf,
			Env:    map[string]string{"COMPOSE_PROJECT_NAME": ""},
		})
		if code != 1 {
			t.Fatalf("expected doctor fail (1) on orphan file, got %d\nstdout=%s", code, out.String())
		}
		if !strings.Contains(out.String(), "unlisted.md") {
			t.Fatalf("expected diagnostic to mention unlisted.md, got: %s", out.String())
		}
	})

	t.Run("catches_broken_feature_link", func(t *testing.T) {
		t.Parallel()
		root := makeSyntheticRepo(t)
		readmePath := filepath.Join(root, ".claude", "skills", "verify-alt", "features", "README.md")
		readme := "# Feature Map\n\n- [sample.md](sample.md)\n- [missing.md](missing.md)\n"
		if err := os.WriteFile(readmePath, []byte(readme), fsperm.File); err != nil {
			t.Fatalf("write readme: %v", err)
		}
		var out, errBuf bytes.Buffer
		code := verifylive.RunWithOptions(context.Background(), []string{"doctor", "--repo-root", root, "--format", "json"}, verifylive.Options{
			Stdout: &out,
			Stderr: &errBuf,
			Env:    map[string]string{"COMPOSE_PROJECT_NAME": ""},
		})
		if code != 1 {
			t.Fatalf("expected doctor fail (1) on broken link, got %d\nstdout=%s", code, out.String())
		}
		if !strings.Contains(out.String(), "missing.md") {
			t.Fatalf("expected diagnostic to mention missing.md, got: %s", out.String())
		}
	})

	t.Run("catches_missing_required_section", func(t *testing.T) {
		t.Parallel()
		root := makeSyntheticRepo(t)
		samplePath := filepath.Join(root, ".claude", "skills", "verify-alt", "features", "sample.md")
		badDoc := "# Sample\n\n## Sub-features\n- a\n\n## How to get to it\n- b\n"
		if err := os.WriteFile(samplePath, []byte(badDoc), fsperm.File); err != nil {
			t.Fatalf("write bad feature doc: %v", err)
		}
		var out, errBuf bytes.Buffer
		code := verifylive.RunWithOptions(context.Background(), []string{"doctor", "--repo-root", root, "--format", "json"}, verifylive.Options{
			Stdout: &out,
			Stderr: &errBuf,
			Env:    map[string]string{"COMPOSE_PROJECT_NAME": ""},
		})
		if code != 1 {
			t.Fatalf("expected doctor fail (1) on missing section, got %d\nstdout=%s", code, out.String())
		}
		if !strings.Contains(out.String(), "Gotchas") {
			t.Fatalf("expected diagnostic to mention Gotchas, got: %s", out.String())
		}
	})

	t.Run("catches_broken_agent_symlink", func(t *testing.T) {
		t.Parallel()
		root := makeSyntheticRepo(t)
		if err := os.Remove(filepath.Join(root, ".agents", "skills", "verify-alt")); err != nil {
			t.Fatalf("remove symlink: %v", err)
		}
		var out, errBuf bytes.Buffer
		code := verifylive.RunWithOptions(context.Background(), []string{"doctor", "--repo-root", root, "--format", "json"}, verifylive.Options{
			Stdout: &out,
			Stderr: &errBuf,
			Env:    map[string]string{"COMPOSE_PROJECT_NAME": ""},
		})
		if code != 1 {
			t.Fatalf("expected doctor fail (1) on missing symlink, got %d\nstdout=%s", code, out.String())
		}
	})
}

func TestRun_RejectsUnsafeTaskNames(t *testing.T) {
	t.Parallel()
	root := makeSyntheticRepo(t)
	evidenceDir := t.TempDir()

	badTasks := []string{
		"",
		"../escape",
		"foo/../bar",
		"a..b",
		"a/b",
		`a\b`,
		".hidden",
		"-leading-dash",
		"has space",
		strings.Repeat("a", 65),
	}
	for _, task := range badTasks {
		t.Run("task_"+task, func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			code := verifylive.RunWithOptions(context.Background(), []string{
				"run", "--task", task, "--probe", "sse", "--repo-root", root, "--evidence-dir", evidenceDir,
			}, verifylive.Options{
				Stdout: &out,
				Stderr: &errBuf,
				Env:    map[string]string{"COMPOSE_PROJECT_NAME": ""},
			})
			if code != 2 {
				t.Fatalf("task %q: got code %d, want 2\nstdout=%s\nstderr=%s", task, code, out.String(), errBuf.String())
			}
		})
	}
}

func TestRun_FailClosedOnProductionProject(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	evidenceDir := t.TempDir()
	var out, errBuf bytes.Buffer
	code := verifylive.RunWithOptions(context.Background(), []string{
		"run", "--task", "prod-guard-check", "--probe", "sse", "--repo-root", root, "--evidence-dir", evidenceDir, "--format", "json",
	}, verifylive.Options{
		Stdout: &out,
		Stderr: &errBuf,
		Env:    map[string]string{"COMPOSE_PROJECT_NAME": "alt"},
	})
	if code != 1 {
		t.Fatalf("expected fail-closed exit 1 for COMPOSE_PROJECT_NAME=alt, got %d\nstdout=%s\nstderr=%s", code, out.String(), errBuf.String())
	}
}

func TestCheckC07Corpus_DetectsBuggyAfterAndPassesFixedBefore(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	beforePath := filepath.Join(root, "scripts", "harness", "corpus", "C-07", "before.py")
	afterPath := filepath.Join(root, "scripts", "harness", "corpus", "C-07", "after.py")

	t.Run("before_passes_and_after_fails", func(t *testing.T) {
		t.Parallel()
		var out, errBuf bytes.Buffer
		code := verifylive.RunWithOptions(context.Background(), []string{
			"check-c07-corpus", "--before", beforePath, "--after", afterPath, "--format", "json",
		}, verifylive.Options{
			Stdout: &out,
			Stderr: &errBuf,
		})
		if code != 0 {
			t.Fatalf("check-c07-corpus exited %d, want 0\nstdout=%s\nstderr=%s", code, out.String(), errBuf.String())
		}
		var res verifylive.C07CorpusResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("unmarshal C07CorpusResult: %v\nraw=%s", err, out.String())
		}
		if !res.Detected || !res.BeforePassed || res.AfterPassed {
			t.Fatalf("unexpected C07 result: %+v", res)
		}
		if res.Signal == "" {
			t.Fatalf("expected non-empty failure signal for C-07 after.py: %+v", res)
		}
	})

	t.Run("fails_when_after_is_not_buggy", func(t *testing.T) {
		t.Parallel()
		var out, errBuf bytes.Buffer
		code := verifylive.RunWithOptions(context.Background(), []string{
			"check-c07-corpus", "--before", beforePath, "--after", beforePath, "--format", "json",
		}, verifylive.Options{
			Stdout: &out,
			Stderr: &errBuf,
		})
		if code != 1 {
			t.Fatalf("expected exit 1 when after.py does not fail live probe, got %d\nstdout=%s", code, out.String())
		}
	})
}

func TestRun_LiveProbesEndToEnd(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	evidenceDir := t.TempDir()

	var out, errBuf bytes.Buffer
	code := verifylive.RunWithOptions(context.Background(), []string{
		"run",
		"--task", "test-live-all",
		"--probe", "all",
		"--check-c07-mutation",
		"--repo-root", root,
		"--evidence-dir", evidenceDir,
		"--format", "json",
	}, verifylive.Options{
		Stdout: &out,
		Stderr: &errBuf,
		Env:    map[string]string{"COMPOSE_PROJECT_NAME": ""},
	})
	if code != 0 {
		t.Fatalf("live probes failed with exit %d\nstdout=%s\nstderr=%s", code, out.String(), errBuf.String())
	}

	var summary verifylive.RunSummary
	if err := json.Unmarshal(out.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal RunSummary: %v\nraw=%s", err, out.String())
	}
	if !summary.OK || !summary.Doctor.OK {
		t.Fatalf("expected summary.OK && summary.Doctor.OK, got: %+v", summary)
	}
	if len(summary.Probes) != 4 {
		t.Fatalf("expected 4 probes, got %d: %+v", len(summary.Probes), summary.Probes)
	}

	taskDir := filepath.Join(evidenceDir, "test-live-all")
	for _, mustExist := range []string{
		"doctor.json",
		"summary.json",
		"sse.json",
		"sse.log",
		"body-limit.json",
		"body-limit.log",
		"auth-hub.json",
		"auth-hub.log",
		"inference-proxy.json",
		"inference-proxy.log",
	} {
		st, err := os.Stat(filepath.Join(taskDir, mustExist))
		if err != nil || st.Size() == 0 {
			t.Fatalf("expected non-empty evidence file %s (err=%v)", mustExist, err)
		}
	}
	for _, p := range summary.Probes {
		if !p.OK {
			t.Fatalf("probe %s failed: %+v", p.Name, p)
		}
		if !strings.HasPrefix(p.BindAddr, "http://127.0.0.1:") {
			t.Fatalf("probe %s bound non-loopback addr %q", p.Name, p.BindAddr)
		}
		if p.Name == "sse" && (p.C07Mutation == nil || !p.C07Mutation.Detected) {
			t.Fatalf("expected sse probe C07Mutation detected=true, got %+v", p.C07Mutation)
		}
	}
}

func TestCLI_HelpAndInvalidUsage(t *testing.T) {
	t.Parallel()
	var out, errBuf bytes.Buffer
	if code := verifylive.RunWithOptions(context.Background(), []string{"--help"}, verifylive.Options{Stdout: &out, Stderr: &errBuf}); code != 0 {
		t.Fatalf("--help got %d, want 0", code)
	}
	if code := verifylive.RunWithOptions(context.Background(), []string{"unknown-sub"}, verifylive.Options{Stdout: &out, Stderr: &errBuf}); code != 2 {
		t.Fatalf("unknown subcommand got %d, want 2", code)
	}
	if code := verifylive.RunWithOptions(context.Background(), []string{"run", "--task", "ok", "--probe", "nonexistent"}, verifylive.Options{Stdout: &out, Stderr: &errBuf}); code != 2 {
		t.Fatalf("unknown probe got %d, want 2", code)
	}
}
