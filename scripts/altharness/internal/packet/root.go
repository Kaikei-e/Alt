package packet

import (
	"os"
	"path/filepath"
	"runtime"
)

func sourceRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	dir := filepath.Dir(file)
	for i := 0; i < 4; i++ {
		dir = filepath.Dir(dir)
	}
	return dir
}

func repoRoot() string {
	if r := sourceRepoRoot(); r != "" {
		if _, err := os.Stat(filepath.Join(r, "scripts", "agent-harness", "task_packet.schema.json")); err == nil {
			return resolveExisting(r)
		}
	}
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for {
			if _, err := os.Stat(filepath.Join(dir, "scripts", "agent-harness", "task_packet.schema.json")); err == nil {
				return resolveExisting(dir)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return resolveExisting(sourceRepoRoot())
}

func resolveExisting(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func harnessDir() string {
	return filepath.Join(repoRoot(), "scripts", "agent-harness")
}

func schemaPath() string {
	return filepath.Join(harnessDir(), "task_packet.schema.json")
}

func packetTemplatePath() string {
	return filepath.Join(harnessDir(), "templates", "task-packet.md.tmpl")
}

func handoffTemplatePath() string {
	return filepath.Join(harnessDir(), "templates", "task-handoff.md.tmpl")
}
