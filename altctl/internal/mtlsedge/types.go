// Package mtlsedge implements ADR-000995 read-only mTLS edge checks for pre-processor -> news-creator.
package mtlsedge

import (
	"fmt"
	"strings"
	"time"

	"github.com/alt-project/altctl/internal/compose"
)

const (
	SchemaVersion           = 1
	EdgeName                = "pre-processor->news-creator"
	CallerService           = "pre-processor"
	ReceiverService         = "news-creator"
	ReceiverURL             = "https://news-creator:9443"
	ProbePath               = "/queue/status"
	ProbeURL                = ReceiverURL + ProbePath
	ComposeProjectName      = "alt"
	AggregateComposeRelPath = "compose/compose.yaml"
	CallerVolumeUIDGID      = "65532:65532"
	DefaultCallerSNI        = "news-creator"
	ExpectedCertFile        = "/certs/svc-cert.pem"
	ExpectedKeyFile         = "/certs/svc-key.pem"
	ExpectedCAFile          = "/trust/ca-bundle.pem"

	StatusPass        = "pass"
	StatusFail        = "fail"
	StatusNotObserved = "not_observed"
)

// ExecutorFactory builds a compose.Executor bound to a specific working
// directory and explicit environment variable slice.
type ExecutorFactory func(workDir string, env []string) compose.Executor

// ToolVersions records tool versions observed during a check.
type ToolVersions struct {
	DockerCompose string `json:"docker_compose"`
}

// CandidateValidateStage holds pre-mutation candidate render validation evidence.
type CandidateValidateStage struct {
	Status             string   `json:"status"`
	CheckedKeys        []string `json:"checked_keys"`
	Reasons            []string `json:"reasons"`
	CandidateEnvSHA256 string   `json:"candidate_env_sha256"`
}

// LiveObservationStage holds post-rollout live container and mTLS probe evidence.
type LiveObservationStage struct {
	Status              string   `json:"status"`
	CallerImageDigest   string   `json:"caller_image_digest"`
	ReceiverImageDigest string   `json:"receiver_image_digest"`
	EnvDriftKeys        []string `json:"env_drift_keys"`
	Probe               string   `json:"probe"`
	Reasons             []string `json:"reasons"`
}

// Stages groups the stage evidence records.
type Stages struct {
	CandidateValidate *CandidateValidateStage `json:"candidate_validate,omitempty"`
	LiveObservation   *LiveObservationStage   `json:"live_observation,omitempty"`
}

// Evidence is the versioned, secret-free evidence object emitted by --json.
type Evidence struct {
	SchemaVersion    int          `json:"schema_version"`
	Edge             string       `json:"edge"`
	DockerContext    string       `json:"docker_context"`
	ToolVersions     ToolVersions `json:"tool_versions"`
	ElapsedMs        int64        `json:"elapsed_ms"`
	RolloutSucceeded bool         `json:"rollout_succeeded"`
	Stages           Stages       `json:"stages"`
}

// Result wraps the Evidence object and reachability state for CLI exit mapping.
type Result struct {
	Evidence        Evidence
	DockerReachable bool
}

func newResult(reachable bool, dockerCtx, composeVer string, start time.Time, rolloutOK bool, stages Stages) *Result {
	return &Result{
		DockerReachable: reachable,
		Evidence: Evidence{
			SchemaVersion:    SchemaVersion,
			Edge:             EdgeName,
			DockerContext:    dockerCtx,
			ToolVersions:     ToolVersions{DockerCompose: composeVer},
			ElapsedMs:        time.Since(start).Milliseconds(),
			RolloutSucceeded: rolloutOK,
			Stages:           stages,
		},
	}
}

// Passed reports whether the executed check observed a clean pass.
func (r *Result) Passed() bool {
	if cv := r.Evidence.Stages.CandidateValidate; cv != nil {
		return cv.Status == StatusPass
	}
	if lo := r.Evidence.Stages.LiveObservation; lo != nil {
		return lo.Status == StatusPass && r.Evidence.RolloutSucceeded
	}
	return false
}

// RenderText formats a human-readable summary of the check without any raw values.
func (r *Result) RenderText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mTLS Edge Check: %s\n", r.Evidence.Edge)
	fmt.Fprintf(&b, "Docker Context:  %s\n", r.Evidence.DockerContext)
	fmt.Fprintf(&b, "Docker Compose:  %s\n", r.Evidence.ToolVersions.DockerCompose)

	if cv := r.Evidence.Stages.CandidateValidate; cv != nil {
		fmt.Fprintf(&b, "Stage:           candidate_validate\n")
		fmt.Fprintf(&b, "Status:          %s\n", cv.Status)
		fmt.Fprintf(&b, "Env SHA-256:     %s\n", cv.CandidateEnvSHA256)
		fmt.Fprintf(&b, "Checked Keys:    %s\n", strings.Join(cv.CheckedKeys, ", "))
		if len(cv.Reasons) > 0 {
			fmt.Fprintf(&b, "Reasons:         %s\n", strings.Join(cv.Reasons, ", "))
		}
	}

	if lo := r.Evidence.Stages.LiveObservation; lo != nil {
		fmt.Fprintf(&b, "Stage:           live_observation\n")
		fmt.Fprintf(&b, "Status:          %s\n", lo.Status)
		fmt.Fprintf(&b, "Rollout OK:      %t\n", r.Evidence.RolloutSucceeded)
		fmt.Fprintf(&b, "Caller Digest:   %s\n", lo.CallerImageDigest)
		fmt.Fprintf(&b, "Receiver Digest: %s\n", lo.ReceiverImageDigest)
		fmt.Fprintf(&b, "Probe:           %s\n", lo.Probe)
		if len(lo.EnvDriftKeys) > 0 {
			fmt.Fprintf(&b, "Env Drift Keys:  %s\n", strings.Join(lo.EnvDriftKeys, ", "))
		}
		if len(lo.Reasons) > 0 {
			fmt.Fprintf(&b, "Reasons:         %s\n", strings.Join(lo.Reasons, ", "))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// ValidateOptions configures `altctl mtls validate`.
type ValidateOptions struct {
	ProjectDir       string
	CandidateEnvFile string
	Environ          []string
	NewExecutor      ExecutorFactory
}

// ObserveOptions configures `altctl mtls observe`.
type ObserveOptions struct {
	ProjectDir          string
	ExpectCallerImage   string
	ExpectReceiverImage string
	Environ             []string
	NewExecutor         ExecutorFactory
}
