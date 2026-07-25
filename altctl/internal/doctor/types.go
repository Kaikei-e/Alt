// Package doctor implements the read-only diagnosis behind `altctl doctor`.
//
// Doctor never changes state: it only shells out to `docker` for
// introspection (info, compose ps, compose config, compose logs) and reads
// files on disk (.env, secrets/, compose/*.yaml). All docker invocations go
// through the compose.Executor interface so the whole package is testable
// against a fake command runner (see doctor_test.go) without a real Docker
// daemon.
package doctor

import (
	"time"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/stack"
)

// Severity classifies how urgently a Finding needs attention.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Service state classification labels (see classify.go).
const (
	StateMissing       = "missing"
	StateUnhealthy     = "unhealthy"
	StateRestarting    = "restarting"
	StateExitedNonZero = "exited_non_zero"
	StateStarting      = "starting"
	StateHealthy       = "healthy"
)

// Finding is a single diagnosis result: an environment problem, a service in
// a bad state, or a static config landmine (e.g. depends_on: service_healthy
// pointing at a service with no healthcheck).
type Finding struct {
	Severity Severity `json:"severity"`
	// Category is "preflight" (environment/daemon), "service" (a service's
	// runtime state), or "config" (a static compose-config landmine).
	Category string `json:"category"`
	Stack    string `json:"stack,omitempty"`
	Service  string `json:"service,omitempty"`
	// State is the classification label (see the State* constants) for
	// Category == "service" findings.
	State   string `json:"state,omitempty"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	// RootCause names the service that is the actual root of this finding
	// when it differs from Service (this service is down only because
	// RootCause is down).
	RootCause string `json:"root_cause,omitempty"`
	// Evidence holds the last log lines captured for this service, oldest
	// first.
	Evidence []string `json:"evidence,omitempty"`
	// Prescription is a list of concrete next commands to run.
	Prescription []string `json:"prescription,omitempty"`
}

// StackReport summarizes one stack's diagnosis.
type StackReport struct {
	Name         string    `json:"name"`
	Optional     bool      `json:"optional"`
	Healthy      bool      `json:"healthy"`
	ServiceCount int       `json:"service_count"`
	Findings     []Finding `json:"findings,omitempty"`
}

// Report is the full result of a doctor run.
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	// Scope lists the stack names that were actually diagnosed.
	Scope []string `json:"scope"`
	// DockerReachable is false when the docker daemon itself could not be
	// contacted -- in that case Stacks is empty and Preflight explains why,
	// loudly, instead of every stack silently looking empty/healthy.
	DockerReachable bool          `json:"docker_reachable"`
	Preflight       []Finding     `json:"preflight,omitempty"`
	Stacks          []StackReport `json:"stacks,omitempty"`
	// Problems is every non-info finding across Preflight and Stacks,
	// flattened, for exit-code purposes and quick machine consumption.
	Problems []Finding `json:"problems,omitempty"`
}

// HasProblems reports whether the run found anything worth a non-zero exit.
func (r *Report) HasProblems() bool {
	return len(r.Problems) > 0
}

// Options configures a Diagnose run.
type Options struct {
	Registry   *stack.Registry
	Executor   compose.Executor
	ProjectDir string
	ComposeDir string
	// Stacks narrows the scope to exactly these stack names. Empty means the
	// default scope: all non-optional stacks, plus any optional stack that
	// has at least one container (running or not) visible via `docker
	// compose ps`.
	Stacks []string
	// LogTailLines is the number of log lines to capture as evidence per
	// problem service. Defaults to 30.
	LogTailLines int
}
