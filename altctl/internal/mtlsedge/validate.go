package mtlsedge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Validate runs the environment freeze and isolated candidate .env render validation.
func Validate(ctx context.Context, opts ValidateOptions) (*Result, error) {
	start := time.Now()
	if opts.ProjectDir == "" {
		return nil, fmt.Errorf("project directory is required")
	}
	if opts.CandidateEnvFile == "" {
		return nil, fmt.Errorf("candidate env file is required")
	}
	if opts.NewExecutor == nil {
		return nil, fmt.Errorf("executor factory is required")
	}
	if opts.Environ == nil {
		return nil, fmt.Errorf("process environment is required")
	}

	projectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}

	candBytes, err := os.ReadFile(opts.CandidateEnvFile)
	if err != nil {
		return nil, fmt.Errorf("read candidate env file: %w", err)
	}
	sum := sha256.Sum256(candBytes)
	candSHA256 := hex.EncodeToString(sum[:])

	composeRoot := filepath.Join(projectDir, AggregateComposeRelPath)
	frozenEnv, overrideReasons, err := FreezeEnvironment(composeRoot, opts.Environ)
	if err != nil {
		return nil, err
	}

	if len(overrideReasons) > 0 {
		stage := &CandidateValidateStage{
			Status:             StatusFail,
			CheckedKeys:        CheckedKeys(),
			Reasons:            overrideReasons,
			CandidateEnvSHA256: candSHA256,
		}
		return newResult(true, StatusNotObserved, StatusNotObserved, start, false, Stages{CandidateValidate: stage}), nil
	}

	rootExec := opts.NewExecutor(projectDir, frozenEnv)
	reachable, dockerCtx, composeVer := probeDockerAndMeta(ctx, rootExec)
	if !reachable {
		stage := &CandidateValidateStage{
			Status:             StatusNotObserved,
			CheckedKeys:        CheckedKeys(),
			Reasons:            []string{"docker_unreachable"},
			CandidateEnvSHA256: candSHA256,
		}
		return newResult(false, StatusNotObserved, StatusNotObserved, start, false, Stages{CandidateValidate: stage}), nil
	}

	stageDir, err := prepareStagingDir(projectDir, candBytes)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(stageDir)
	}()

	stageComposeFile := filepath.Join(stageDir, AggregateComposeRelPath)
	stageEnvFile := filepath.Join(stageDir, ".env")
	stageExec := opts.NewExecutor(stageDir, frozenEnv)

	callerEnv, receiverEnv, reasons := renderComposeServiceEnvs(ctx, stageExec, stageComposeFile, stageEnvFile)
	if len(reasons) == 0 {
		reasons = CheckSemanticEdge(callerEnv, receiverEnv)
	}

	status := StatusPass
	if len(reasons) > 0 {
		status = StatusFail
	}

	stage := &CandidateValidateStage{
		Status:             status,
		CheckedKeys:        CheckedKeys(),
		Reasons:            reasons,
		CandidateEnvSHA256: candSHA256,
	}
	return newResult(true, dockerCtx, composeVer, start, false, Stages{CandidateValidate: stage}), nil
}

func prepareStagingDir(absRoot string, candidateEnvBytes []byte) (string, error) {
	stageDir, err := os.MkdirTemp("", "altctl-mtls-validate-*")
	if err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}

	entries, err := os.ReadDir(absRoot)
	if err != nil {
		_ = os.RemoveAll(stageDir)
		return "", fmt.Errorf("read project root: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == ".env" || name == "compose" {
			continue
		}
		if err := os.Symlink(filepath.Join(absRoot, name), filepath.Join(stageDir, name)); err != nil {
			_ = os.RemoveAll(stageDir)
			return "", fmt.Errorf("symlink %s into staging dir: %w", name, err)
		}
	}

	srcComposeDir := filepath.Join(absRoot, "compose")
	dstComposeDir := filepath.Join(stageDir, "compose")
	if err := copyComposeDirBytes(srcComposeDir, dstComposeDir); err != nil {
		_ = os.RemoveAll(stageDir)
		return "", err
	}

	if err := os.WriteFile(filepath.Join(stageDir, ".env"), candidateEnvBytes, 0o600); err != nil {
		_ = os.RemoveAll(stageDir)
		return "", fmt.Errorf("write staged .env: %w", err)
	}

	if err := os.Symlink("../.env", filepath.Join(dstComposeDir, ".env")); err != nil {
		_ = os.RemoveAll(stageDir)
		return "", fmt.Errorf("create compose/.env symlink: %w", err)
	}

	return stageDir, nil
}

func copyComposeDirBytes(srcComposeDir, dstComposeDir string) error {
	return filepath.WalkDir(srcComposeDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcComposeDir, path)
		if err != nil {
			return err
		}
		if rel == ".env" {
			return nil
		}
		dstPath := filepath.Join(dstComposeDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o750)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read compose file %s: %w", rel, err)
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat compose file %s: %w", rel, err)
		}
		if err := os.WriteFile(dstPath, raw, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write staged compose file %s: %w", rel, err)
		}
		return nil
	})
}
