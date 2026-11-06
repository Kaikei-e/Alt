package mtlsedge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/alt-project/altctl/internal/compose"
)

type containerInspectRecord struct {
	ID     string `json:"Id"`
	Image  string `json:"Image"`
	Config struct {
		Env []string `json:"Env"`
	} `json:"Config"`
	Mounts []struct {
		Type        string `json:"Type"`
		Name        string `json:"Name"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

type imageInspectRecord struct {
	ID          string   `json:"Id"`
	RepoDigests []string `json:"RepoDigests"`
}

type exitCoder interface {
	ExitCode() int
}

var expectedImageRefRegex = regexp.MustCompile(`^[^\s@]+@sha256:[0-9a-f]{64}$`)

// ValidateExpectedImageRef checks that a non-empty expected image reference has
// the form <repo>@sha256:<64 lowercase hex>.
func ValidateExpectedImageRef(ref string) error {
	if ref == "" || expectedImageRefRegex.MatchString(ref) {
		return nil
	}
	return fmt.Errorf("expected image must match <repo>@sha256:<64 lowercase hex>")
}

// Observe runs the environment freeze, live container inspection, image digest verification,
// effective env drift comparison, semantic edge validation, and the caller-originated mTLS probe.
func Observe(ctx context.Context, opts ObserveOptions) (*Result, error) {
	start := time.Now()
	if opts.ProjectDir == "" {
		return nil, fmt.Errorf("project directory is required")
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

	composeRoot := filepath.Join(projectDir, AggregateComposeRelPath)
	frozenEnv, overrideReasons, err := FreezeEnvironment(composeRoot, opts.Environ)
	if err != nil {
		return nil, err
	}

	if len(overrideReasons) > 0 {
		stage := &LiveObservationStage{
			Status:              StatusFail,
			CallerImageDigest:   StatusNotObserved,
			ReceiverImageDigest: StatusNotObserved,
			EnvDriftKeys:        []string{},
			Probe:               StatusNotObserved,
			Reasons:             overrideReasons,
		}
		return newResult(true, StatusNotObserved, StatusNotObserved, start, false, Stages{LiveObservation: stage}), nil
	}

	runner := opts.NewExecutor(projectDir, frozenEnv)
	reachable, dockerCtx, composeVer := probeDockerAndMeta(ctx, runner)
	if !reachable {
		stage := &LiveObservationStage{
			Status:              StatusNotObserved,
			CallerImageDigest:   StatusNotObserved,
			ReceiverImageDigest: StatusNotObserved,
			EnvDriftKeys:        []string{},
			Probe:               StatusNotObserved,
			Reasons:             []string{"docker_unreachable"},
		}
		return newResult(false, StatusNotObserved, StatusNotObserved, start, false, Stages{LiveObservation: stage}), nil
	}

	callerCtr, callerReason, callerFailClosed := inspectSingleRunningServiceContainer(ctx, runner, CallerService, "caller")
	receiverCtr, receiverReason, receiverFailClosed := inspectSingleRunningServiceContainer(ctx, runner, ReceiverService, "receiver")
	if callerReason != "" || receiverReason != "" {
		reasons := make([]string, 0, 2)
		if callerReason != "" {
			reasons = append(reasons, callerReason)
		}
		if receiverReason != "" {
			reasons = append(reasons, receiverReason)
		}
		status := StatusNotObserved
		if callerFailClosed || receiverFailClosed {
			status = StatusFail
		}
		stage := &LiveObservationStage{
			Status:              status,
			CallerImageDigest:   StatusNotObserved,
			ReceiverImageDigest: StatusNotObserved,
			EnvDriftKeys:        []string{},
			Probe:               StatusNotObserved,
			Reasons:             reasons,
		}
		return newResult(true, dockerCtx, composeVer, start, false, Stages{LiveObservation: stage}), nil
	}

	reasons := make([]string, 0)
	envDriftKeys := make([]string, 0)

	callerDigest, callerDigestReasons := resolveAndVerifyImageDigest(ctx, runner, callerCtr.Image, opts.ExpectCallerImage, "caller")
	reasons = append(reasons, callerDigestReasons...)

	receiverDigest, receiverDigestReasons := resolveAndVerifyImageDigest(ctx, runner, receiverCtr.Image, opts.ExpectReceiverImage, "receiver")
	reasons = append(reasons, receiverDigestReasons...)

	callerCtrEnv := parseEnvSlice(callerCtr.Config.Env)
	receiverCtrEnv := parseEnvSlice(receiverCtr.Config.Env)

	renderedCallerEnv, renderedReceiverEnv, renderReasons := renderComposeServiceEnvs(ctx, runner, composeRoot)
	if len(renderReasons) > 0 {
		reasons = append(reasons, renderReasons...)
	} else {
		for _, k := range CallerEdgeKeys {
			if renderedCallerEnv[k] != callerCtrEnv[k] {
				envDriftKeys = append(envDriftKeys, k)
				reasons = append(reasons, "env_drift:"+k)
			}
		}
		for _, k := range ReceiverEdgeKeys {
			if renderedReceiverEnv[k] != receiverCtrEnv[k] {
				envDriftKeys = append(envDriftKeys, k)
				reasons = append(reasons, "env_drift:"+k)
			}
		}
	}

	reasons = append(reasons, CheckSemanticEdge(callerCtrEnv, receiverCtrEnv)...)

	probeStatus, probeReason := runCallerOriginMTLSProbe(ctx, runner, callerCtr, receiverCtr.Image)
	if probeReason != "" {
		reasons = append(reasons, probeReason)
	}

	overallStatus := StatusPass
	if len(reasons) > 0 || probeStatus != StatusPass {
		if probeStatus == StatusNotObserved && !hasHardFailReason(reasons) {
			overallStatus = StatusNotObserved
		} else {
			overallStatus = StatusFail
		}
	}

	rolloutSucceeded := overallStatus == StatusPass
	stage := &LiveObservationStage{
		Status:              overallStatus,
		CallerImageDigest:   callerDigest,
		ReceiverImageDigest: receiverDigest,
		EnvDriftKeys:        envDriftKeys,
		Probe:               probeStatus,
		Reasons:             reasons,
	}
	return newResult(true, dockerCtx, composeVer, start, rolloutSucceeded, Stages{LiveObservation: stage}), nil
}

func hasHardFailReason(reasons []string) bool {
	for _, r := range reasons {
		if r != "probe_container_error" {
			return true
		}
	}
	return false
}

func inspectSingleRunningServiceContainer(ctx context.Context, runner compose.Executor, service, role string) (*containerInspectRecord, string, bool) {
	var out, errBuf bytes.Buffer
	args := []string{
		"ps",
		"-q",
		"--filter", "label=com.docker.compose.project=" + ComposeProjectName,
		"--filter", "label=com.docker.compose.service=" + service,
		"--filter", "status=running",
	}
	if err := runner.RunWithPipes(ctx, "docker", args, &out, &errBuf); err != nil {
		return nil, role + "_container_not_single_running", false
	}
	ids := strings.Fields(out.String())
	if len(ids) != 1 {
		return nil, role + "_container_not_single_running", false
	}

	out.Reset()
	errBuf.Reset()
	if err := runner.RunWithPipes(ctx, "docker", []string{"container", "inspect", ids[0]}, &out, &errBuf); err != nil {
		return nil, role + "_container_not_single_running", false
	}

	var records []containerInspectRecord
	if err := json.Unmarshal(out.Bytes(), &records); err != nil || len(records) != 1 {
		return nil, role + "_container_not_single_running", false
	}
	rec := records[0]
	if strings.TrimSpace(rec.ID) == "" {
		return nil, role + "_container_missing_id", true
	}
	return &rec, "", false
}

func resolveAndVerifyImageDigest(ctx context.Context, runner compose.Executor, imageID, expectedRef, role string) (string, []string) {
	var out, errBuf bytes.Buffer
	if err := runner.RunWithPipes(ctx, "docker", []string{"image", "inspect", imageID}, &out, &errBuf); err != nil {
		return StatusNotObserved, []string{role + "_image_inspect_failed"}
	}

	var imgs []imageInspectRecord
	if err := json.Unmarshal(out.Bytes(), &imgs); err != nil || len(imgs) != 1 {
		return StatusNotObserved, []string{role + "_image_inspect_parse_failed"}
	}

	repoDigests := imgs[0].RepoDigests
	if len(repoDigests) == 0 {
		return StatusNotObserved, []string{role + "_image_no_repodigest"}
	}

	observed := repoDigests[0]
	if expectedRef == "" {
		return observed, nil
	}
	if slices.Contains(repoDigests, expectedRef) {
		return expectedRef, nil
	}
	return observed, []string{role + "_image_digest_mismatch"}
}

func runCallerOriginMTLSProbe(ctx context.Context, runner compose.Executor, caller *containerInspectRecord, receiverImageID string) (string, string) {
	certsVol := findVolumeMountName(caller, "/certs")
	trustVol := findVolumeMountName(caller, "/trust")
	if certsVol == "" || trustVol == "" {
		return StatusFail, "caller_mount_missing:/certs,/trust"
	}

	args := []string{
		"run",
		"--rm",
		"--pull", "never",
		"--network", "container:" + caller.ID,
		"--user", CallerVolumeUIDGID,
		"--mount", "type=volume,src=" + certsVol + ",dst=/certs,ro",
		"--mount", "type=volume,src=" + trustVol + ",dst=/trust,ro",
		"--entrypoint", "curl",
		receiverImageID,
		"-sS",
		"-o", "/dev/null",
		"--max-time", "10",
		"--fail",
		"--cert", ExpectedCertFile,
		"--key", ExpectedKeyFile,
		"--cacert", ExpectedCAFile,
		ProbeURL,
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	err := runner.RunWithPipes(ctx, "docker", args, &stdoutBuf, &stderrBuf)
	if err == nil {
		return StatusPass, ""
	}

	code := extractExitCode(err)
	switch {
	case code == 22:
		return StatusFail, "probe_http_error"
	case code >= 125 && code <= 127:
		return StatusNotObserved, "probe_container_error"
	default:
		return StatusFail, "probe_tls_or_connect_error"
	}
}

func findVolumeMountName(ctr *containerInspectRecord, destination string) string {
	for _, m := range ctr.Mounts {
		if m.Destination == destination && (m.Type == "volume" || m.Type == "") && strings.TrimSpace(m.Name) != "" {
			return strings.TrimSpace(m.Name)
		}
	}
	return ""
}

func extractExitCode(err error) int {
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 1
}

func parseEnvSlice(envSlice []string) map[string]string {
	out := make(map[string]string, len(envSlice))
	for _, item := range envSlice {
		k, v, _ := strings.Cut(item, "=")
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}
