package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	protoModuleDir = "../.."
	repoRootDir    = "../../.."
)

// buildRepoDescriptorSet compiles the real proto module the same way the
// generator and CI do. buf is not a test dependency of this module, so a
// machine without it skips rather than fails.
func buildRepoDescriptorSet(t *testing.T) *descriptorpb.FileDescriptorSet {
	t.Helper()

	buf, err := exec.LookPath("buf")
	if err != nil {
		t.Skipf("buf not on PATH: %v", err)
	}

	out := filepath.Join(t.TempDir(), "descriptor.binpb")
	cmd := exec.Command(buf, "build", "--exclude-source-info", "-o", out)
	cmd.Dir = protoModuleDir
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("buf build: %v\n%s", err, combined)
	}

	fds, err := LoadDescriptorSet(out)
	if err != nil {
		t.Fatalf("LoadDescriptorSet() error = %v", err)
	}
	return fds
}

func TestRepoProtosDeclareVisibility(t *testing.T) {
	list, err := Classify(buildRepoDescriptorSet(t))
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if len(list.Public) == 0 || len(list.Admin) == 0 {
		t.Fatalf("Classify() = %+v, want both roots populated", list)
	}
}

// TestCommittedArtifactsAreCurrent is the local half of the CI drift gate: the
// checked-in allowlists must be exactly what the current protos render to.
func TestCommittedArtifactsAreCurrent(t *testing.T) {
	list, err := Classify(buildRepoDescriptorSet(t))
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	tests := []struct {
		name   string
		path   string
		render func(Allowlist) ([]byte, error)
	}{
		{name: "bff go allowlist", path: defaultGoOut, render: RenderGo},
		{name: "frontend ts allowlist", path: defaultTSOut, render: RenderTS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := tt.render(list)
			if err != nil {
				t.Fatalf("render error = %v", err)
			}
			got, err := os.ReadFile(filepath.Join(repoRootDir, tt.path))
			if err != nil {
				t.Fatalf("read committed artifact: %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("%s is stale; re-run protovis\n--- committed ---\n%s\n--- generated ---\n%s", tt.path, got, want)
			}
		})
	}
}
