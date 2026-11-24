package generated

import (
	"alt/scripts/altharness/internal/codegen"
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/pki"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type openAPITarget struct {
	rel   string
	flags []string
}

var openAPITargets = []openAPITarget{
	{
		rel:   "rag-orchestrator/internal/adapter/rag_http/openapi/server.gen.go",
		flags: []string{"-package", "openapi", "-generate", "types,echo5-server,spec"},
	},
	{
		rel: "alt-backend/app/orchestrator/gateway/rag_gateway/client.gen.go",
		flags: []string{
			"-package", "rag_gateway", "-generate", "types,client",
			"-exclude-operation-ids", "CandidateSimilarity",
		},
	},
}

var protovisOutputs = []string{
	"alt-butterfly-facade/internal/server/allowlist_gen.go",
	"alt-frontend-sv/src/lib/gen/allowlist.ts",
}

// canonMemo remembers one canonical-sync result for the lifetime of a single
// check. It is not a process-wide cache.
type canonMemo struct {
	mu    sync.Mutex
	cache map[string]canonResult
}

type canonResult struct {
	vs  []Violation
	err error
}

func (m *canonMemo) get(key string) (canonResult, bool) {
	if m == nil {
		return canonResult{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hit, ok := m.cache[key]
	return hit, ok
}

func (m *canonMemo) put(key string, hit canonResult) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache == nil {
		m.cache = map[string]canonResult{}
	}
	m.cache[key] = hit
}

func (c *checker) checkRegeneration(ctx context.Context, repoRoot string) ([]Violation, error) {
	root := prepareRoot(repoRoot)
	memo := &canonMemo{}

	type slot struct {
		vs  []Violation
		err error
	}
	var results [4]slot
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		results[0].vs, results[0].err = c.checkGoCanonical(ctx, root, "scripts/codegen/go_canonical", ruleDriftGoCanonical, memo)
	}()
	go func() {
		defer wg.Done()
		results[1].vs, results[1].err = c.checkPythonPKI(ctx, root)
	}()
	go func() {
		defer wg.Done()
		results[2].vs, results[2].err = c.checkOpenAPIRegen(ctx, root)
	}()
	go func() {
		defer wg.Done()
		results[3].vs, results[3].err = c.checkProtovisRegen(ctx, root)
	}()
	wg.Wait()

	var violations []Violation
	for _, res := range results {
		if res.err != nil {
			return nil, res.err
		}
		violations = append(violations, res.vs...)
	}
	return violations, nil
}

func (c *checker) checkGoCanonical(ctx context.Context, repoRoot, dirRel, rule string, memo *canonMemo) ([]Violation, error) {
	key := repoRoot + "\x00" + dirRel + "\x00" + rule
	if hit, ok := memo.get(key); ok {
		return hit.vs, hit.err
	}
	vs, err := c.runGoCanonical(ctx, repoRoot, dirRel, rule)
	memo.put(key, canonResult{vs: vs, err: err})
	return vs, err
}

func (c *checker) runGoCanonical(ctx context.Context, repoRoot, dirRel, rule string) ([]Violation, error) {
	canonicalDir := filepath.Join(repoRoot, filepath.FromSlash(dirRel))
	if !isDir(canonicalDir) {
		return nil, nil
	}
	sctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := codegen.Sync(sctx, repoRoot, true, &stdout, &stderr)
	if ctx.Err() != nil {
		return nil, fmt.Errorf("sync go canonical: %w", ctx.Err())
	}
	if code == 0 {
		return nil, nil
	}
	summary := lastOutputLine(stdout.String(), stderr.String(), fmt.Sprintf("sync-go --check failed (exit %d)", code))
	return []Violation{{
		Path:    dirRel,
		Line:    1,
		Rule:    rule,
		Message: summary,
	}}, nil
}

func (c *checker) checkPythonPKI(ctx context.Context, repoRoot string) ([]Violation, error) {
	const dirRel = "scripts/codegen/pki_py/templates"
	templatesDir := filepath.Join(repoRoot, filepath.FromSlash(dirRel))
	if !isDir(templatesDir) {
		return nil, nil
	}
	sctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := pki.Sync(sctx, repoRoot, true, &stdout, &stderr)
	if ctx.Err() != nil {
		return nil, fmt.Errorf("sync python pki: %w", ctx.Err())
	}
	if code == 0 {
		return nil, nil
	}
	summary := lastOutputLine(stdout.String(), stderr.String(), fmt.Sprintf("pki --check failed (exit %d)", code))
	return []Violation{{
		Path:    dirRel,
		Line:    1,
		Rule:    ruleDriftPythonPKI,
		Message: summary,
	}}, nil
}

func (c *checker) checkOpenAPIRegen(ctx context.Context, repoRoot string) ([]Violation, error) {
	ragDir := filepath.Join(repoRoot, "rag-orchestrator")
	spec := filepath.Join(ragDir, "spec", "openapi.yaml")
	toolMod := filepath.Join(repoRoot, "scripts", "codegen", "go.tool.mod")
	if !isRegularFile(spec) || !isRegularFile(toolMod) {
		return nil, nil
	}
	goBin, err := c.which("go")
	if err != nil {
		return []Violation{{
			Path:    "rag-orchestrator/spec/openapi.yaml",
			Line:    1,
			Rule:    ruleDriftOpenAPI,
			Message: "go binary not found in PATH for oapi-codegen regeneration check",
		}}, nil
	}
	tmp, err := os.MkdirTemp("", "alt-oapi-check-")
	if err != nil {
		return []Violation{{
			Path:    "rag-orchestrator/spec/openapi.yaml",
			Line:    1,
			Rule:    ruleDriftOpenAPI,
			Message: "oapi-codegen execution failed: " + err.Error(),
		}}, nil
	}
	defer os.RemoveAll(tmp)

	type targetResult struct {
		v         *Violation
		missingGo bool
		err       error
	}
	slots := make([]targetResult, len(openAPITargets))
	var wg sync.WaitGroup
	for idx, target := range openAPITargets {
		wg.Add(1)
		go func(idx int, target openAPITarget) {
			defer wg.Done()
			if err := ctx.Err(); err != nil {
				slots[idx].err = fmt.Errorf("openapi regeneration: %w", err)
				return
			}
			outPath := filepath.Join(tmp, fmt.Sprintf("gen_%d.go", idx))
			args := []string{goBin, "-C", ragDir, "tool", "-modfile=" + toolMod, "oapi-codegen"}
			args = append(args, target.flags...)
			args = append(args, "-o", outPath, "spec/openapi.yaml")
			stdout, stderr, code, runErr := c.exec(ctx, args, "")
			if ctx.Err() != nil {
				slots[idx].err = fmt.Errorf("openapi regeneration: %w", ctx.Err())
				return
			}
			if code == 127 {
				slots[idx].missingGo = true
				return
			}
			if code != 0 || runErr != nil {
				slots[idx].v = &Violation{
					Path:    target.rel,
					Line:    1,
					Rule:    ruleDriftOpenAPI,
					Message: "oapi-codegen execution failed: " + outputDetail(stdout, stderr, runErr),
				}
				return
			}
			targetPath := filepath.Join(repoRoot, filepath.FromSlash(target.rel))
			if !isRegularFile(targetPath) {
				slots[idx].v = &Violation{
					Path:    target.rel,
					Line:    1,
					Rule:    ruleDriftOpenAPI,
					Message: "committed generated file missing: " + target.rel,
				}
				return
			}
			actual, readErr := os.ReadFile(targetPath)
			expected, expErr := os.ReadFile(outPath)
			if readErr != nil || expErr != nil || !bytes.Equal(actual, expected) {
				slots[idx].v = &Violation{
					Path: target.rel,
					Line: 1,
					Rule: ruleDriftOpenAPI,
					Message: "generated OpenAPI file drifted from rag-orchestrator/spec/openapi.yaml; " +
						"run `make -C rag-orchestrator generate` (" + target.rel + ")",
				}
			}
		}(idx, target)
	}
	wg.Wait()

	var violations []Violation
	for _, res := range slots {
		if res.err != nil {
			return nil, res.err
		}
		if res.missingGo {
			return []Violation{{
				Path:    "rag-orchestrator/spec/openapi.yaml",
				Line:    1,
				Rule:    ruleDriftOpenAPI,
				Message: "go binary not found in PATH for oapi-codegen regeneration check",
			}}, nil
		}
		if res.v != nil {
			violations = append(violations, *res.v)
		}
	}
	return violations, nil
}

func (c *checker) checkProtovisRegen(ctx context.Context, repoRoot string) ([]Violation, error) {
	protoDir := filepath.Join(repoRoot, "proto")
	protovisDir := filepath.Join(protoDir, "tools", "protovis")
	if !isRegularFile(filepath.Join(protoDir, "buf.yaml")) || !isRegularFile(filepath.Join(protovisDir, "go.mod")) {
		return nil, nil
	}
	bufBin, bufErr := c.which("buf")
	goBin, goErr := c.which("go")
	var missing []Violation
	if bufErr != nil {
		missing = append(missing, Violation{
			Path:    "proto/buf.yaml",
			Line:    1,
			Rule:    ruleRegenToolMissing,
			Message: "buf binary not found in PATH for protovis regeneration check",
		})
	}
	if goErr != nil {
		missing = append(missing, Violation{
			Path:    "proto/tools/protovis",
			Line:    1,
			Rule:    ruleRegenToolMissing,
			Message: "go binary not found in PATH for protovis regeneration check",
		})
	}
	if bufErr != nil || goErr != nil {
		return missing, nil
	}

	tmp, err := os.MkdirTemp("", "alt-protovis-check-")
	if err != nil {
		return []Violation{{
			Path:    "proto/buf.yaml",
			Line:    1,
			Rule:    ruleDriftProtovis,
			Message: "buf build failed: " + err.Error(),
		}}, nil
	}
	defer os.RemoveAll(tmp)
	descPath := filepath.Join(tmp, "descriptor.binpb")
	outRoot := filepath.Join(tmp, "out")
	if err := os.Mkdir(outRoot, fsperm.Dir); err != nil {
		return []Violation{{
			Path:    "proto/buf.yaml",
			Line:    1,
			Rule:    ruleDriftProtovis,
			Message: "buf build failed: " + err.Error(),
		}}, nil
	}
	stdout, stderr, code, runErr := c.exec(ctx, []string{bufBin, "build", "--exclude-source-info", "-o", descPath}, protoDir)
	if ctx.Err() != nil {
		return nil, fmt.Errorf("protovis regeneration: %w", ctx.Err())
	}
	if code != 0 || runErr != nil {
		return []Violation{{
			Path:    "proto/buf.yaml",
			Line:    1,
			Rule:    ruleDriftProtovis,
			Message: "buf build failed: " + outputDetail(stdout, stderr, runErr),
		}}, nil
	}
	stdout, stderr, code, runErr = c.exec(ctx, []string{goBin, "run", ".", "-desc", descPath, "-root", outRoot}, protovisDir)
	if ctx.Err() != nil {
		return nil, fmt.Errorf("protovis regeneration: %w", ctx.Err())
	}
	if code != 0 || runErr != nil {
		return []Violation{{
			Path:    "proto/tools/protovis",
			Line:    1,
			Rule:    ruleDriftProtovis,
			Message: "protovis generator failed: " + outputDetail(stdout, stderr, runErr),
		}}, nil
	}
	var violations []Violation
	for _, rel := range protovisOutputs {
		committed := filepath.Join(repoRoot, filepath.FromSlash(rel))
		regenerated := filepath.Join(outRoot, filepath.FromSlash(rel))
		if !isRegularFile(committed) {
			violations = append(violations, Violation{
				Path:    rel,
				Line:    1,
				Rule:    ruleDriftProtovis,
				Message: "generated allowlist missing: " + rel,
			})
			continue
		}
		actual, readErr := os.ReadFile(committed)
		expected, expErr := os.ReadFile(regenerated)
		if readErr != nil || expErr != nil || !isRegularFile(regenerated) || !bytes.Equal(actual, expected) {
			violations = append(violations, Violation{
				Path:    rel,
				Line:    1,
				Rule:    ruleDriftProtovis,
				Message: "generated allowlist drifted from proto definitions: " + rel,
			})
		}
	}
	return violations, nil
}

func lastOutputLine(stdout, stderr, fallback string) string {
	detail := commandDetail(stdout, stderr)
	lines := splitLines(detail)
	if len(lines) == 0 {
		return fallback
	}
	return lines[len(lines)-1]
}
