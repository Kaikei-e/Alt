package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"protovis/internal/fsperm"
	"strings"
	"syscall"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update effective config golden file")

type protovisEffectiveConfig struct {
	DefaultOutputs  defaultOutputsSnapshot  `json:"default_outputs"`
	EmittedModes    emittedModesSnapshot    `json:"emitted_modes"`
	FSPermConstants fsPermConstantsSnapshot `json:"fsperm_constants"`
	GeneratedHeader string                  `json:"generated_header"`
	Service         string                  `json:"service"`
}

type defaultOutputsSnapshot struct {
	GoOut string `json:"go_out"`
	TSOut string `json:"ts_out"`
}

type emittedModesSnapshot struct {
	DirectoryMode string `json:"directory_mode"`
	GoFileMode    string `json:"go_file_mode"`
	TSFileMode    string `json:"ts_file_mode"`
}

type fsPermConstantsSnapshot struct {
	GeneratedDir  string `json:"generated_dir"`
	GeneratedFile string `json:"generated_file"`
}

const (
	goldenDirPath  = "testdata"
	goldenFilePath = "testdata/effective_config.golden.json"
)

func TestEffectiveConfigGolden(t *testing.T) {
	snap := captureProtovisEffectiveConfig(t, writeArtifact)
	got := mustCanonicalJSON(t, snap)

	if *updateGolden || os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(goldenDirPath, fsperm.GeneratedDir); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenFilePath, got, fsperm.GeneratedFile); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenFilePath)
	if err != nil {
		t.Fatalf("read golden %s (run with -update or UPDATE_GOLDEN=1 to create): %v", goldenFilePath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("effective runtime config drifted from %s:\n%s", goldenFilePath, diffLines(string(want), string(got)))
	}
}

func TestEffectiveConfigGolden_MutationsDetected(t *testing.T) {
	want, err := os.ReadFile(goldenFilePath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenFilePath, err)
	}

	t.Run("C-12 mutation tightening output modes to 0750/0600 produces golden diff", func(t *testing.T) {
		mutatedWriter := func(path string, content []byte) error {
			if err := os.MkdirAll(filepath.Dir(path), fsperm.PrivateDir); err != nil {
				return err
			}
			return os.WriteFile(path, content, fsperm.SecretFile)
		}
		snap := captureProtovisEffectiveConfig(t, mutatedWriter)
		if snap.EmittedModes.DirectoryMode != "0750" || snap.EmittedModes.GoFileMode != "0600" {
			t.Fatalf("expected mutated modes 0750/0600, got dir=%s go=%s",
				snap.EmittedModes.DirectoryMode, snap.EmittedModes.GoFileMode)
		}
		got := mustCanonicalJSON(t, snap)
		if bytes.Equal(got, want) {
			t.Fatal("C-12 tightened permission mutation matched golden; expected diff")
		}
	})
}

func captureProtovisEffectiveConfig(t *testing.T, writer func(string, []byte) error) protovisEffectiveConfig {
	t.Helper()

	list := Allowlist{
		Public: []string{"alt.example.v1.PublicService"},
		Admin:  []string{"alt.example.v1.AdminService"},
	}
	goSrc, err := RenderGo(list)
	if err != nil {
		t.Fatalf("RenderGo: %v", err)
	}
	tsSrc, err := RenderTS(list)
	if err != nil {
		t.Fatalf("RenderTS: %v", err)
	}

	// Zero process umask only while writing to t.TempDir() so os.Stat reflects
	// the exact permission bits requested by writeArtifact, independent of host umask.
	oldUmask := syscall.Umask(0)
	defer syscall.Umask(oldUmask)

	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "emitted")
	goPath := filepath.Join(outDir, "allowlist_gen.go")
	tsPath := filepath.Join(outDir, "allowlist.ts")

	if err := writer(goPath, goSrc); err != nil {
		t.Fatalf("write go artifact: %v", err)
	}
	if err := writer(tsPath, tsSrc); err != nil {
		t.Fatalf("write ts artifact: %v", err)
	}

	dirInfo, err := os.Stat(outDir)
	if err != nil {
		t.Fatalf("stat emitted dir: %v", err)
	}
	goInfo, err := os.Stat(goPath)
	if err != nil {
		t.Fatalf("stat emitted go file: %v", err)
	}
	tsInfo, err := os.Stat(tsPath)
	if err != nil {
		t.Fatalf("stat emitted ts file: %v", err)
	}

	return protovisEffectiveConfig{
		Service: "protovis",
		DefaultOutputs: defaultOutputsSnapshot{
			GoOut: defaultGoOut,
			TSOut: defaultTSOut,
		},
		EmittedModes: emittedModesSnapshot{
			DirectoryMode: fmt.Sprintf("%04o", dirInfo.Mode().Perm()),
			GoFileMode:    fmt.Sprintf("%04o", goInfo.Mode().Perm()),
			TSFileMode:    fmt.Sprintf("%04o", tsInfo.Mode().Perm()),
		},
		FSPermConstants: fsPermConstantsSnapshot{
			GeneratedDir:  fmt.Sprintf("%04o", fsperm.GeneratedDir),
			GeneratedFile: fmt.Sprintf("%04o", fsperm.GeneratedFile),
		},
		GeneratedHeader: generatedHeader,
	}
}

func mustCanonicalJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var parsed any
	if err := dec.Decode(&parsed); err != nil {
		t.Fatalf("json.Decode: %v", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(parsed); err != nil {
		t.Fatalf("json.Encode: %v", err)
	}
	return buf.Bytes()
}

func diffLines(want, got string) string {
	wLines := strings.Split(strings.TrimRight(want, "\n"), "\n")
	gLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	n := len(wLines)
	if len(gLines) > n {
		n = len(gLines)
	}
	var b strings.Builder
	for i := range n {
		var wl, gl string
		hasW, hasG := i < len(wLines), i < len(gLines)
		if hasW {
			wl = wLines[i]
		}
		if hasG {
			gl = gLines[i]
		}
		if hasW && hasG && wl == gl {
			continue
		}
		if hasW {
			fmt.Fprintf(&b, "- %s\n", wl)
		}
		if hasG {
			fmt.Fprintf(&b, "+ %s\n", gl)
		}
	}
	return b.String()
}
