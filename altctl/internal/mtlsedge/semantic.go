package mtlsedge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/alt-project/altctl/internal/compose"
)

var callerForbiddenFileKeys = []string{
	"CERT_SUBJECT_FILE",
	"CERT_PATH_FILE",
	"KEY_PATH_FILE",
}

var receiverForbiddenFileKeys = []string{
	"CERT_SANS_FILE",
	"CERT_SUBJECT_FILE",
	"CERT_PATH_FILE",
	"KEY_PATH_FILE",
}

// CallerEdgeKeys lists the environment keys on pre-processor that wire.go and PKI enrollment
// read for the news-creator mTLS edge, including PKI _FILE overrides that must be absent.
var CallerEdgeKeys = []string{
	"MTLS_ENFORCE",
	"MTLS_CERT_FILE",
	"MTLS_KEY_FILE",
	"MTLS_CA_FILE",
	"NEWS_CREATOR_HOST",
	"NEWS_CREATOR_MTLS_SERVER_NAME",
	"CERT_SUBJECT",
	"CERT_SUBJECT_FILE",
	"CERT_PATH_FILE",
	"KEY_PATH_FILE",
}

// ReceiverEdgeKeys lists the environment keys on news-creator that inbound TLS,
// peer-identity middleware, and PKI enrollment read for the mTLS edge, including
// PKI _FILE overrides that must be absent.
var ReceiverEdgeKeys = []string{
	"INBOUND_MTLS",
	"PEER_IDENTITY_STRICT",
	"CERT_SANS",
	"MTLS_ALLOWED_PEERS",
	"CERT_SANS_FILE",
	"CERT_SUBJECT_FILE",
	"CERT_PATH_FILE",
	"KEY_PATH_FILE",
}

// CheckedKeys returns a copy of all effective environment keys inspected by the semantic edge check.
func CheckedKeys() []string {
	keys := make([]string, 0, len(CallerEdgeKeys)+len(ReceiverEdgeKeys))
	keys = append(keys, CallerEdgeKeys...)
	for _, k := range ReceiverEdgeKeys {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// CheckSemanticEdge verifies the pre-processor -> news-creator mTLS edge contract
// against both services' effective environment maps. Every failure yields a stable
// reason identifier naming the key(s) only, never a value.
func CheckSemanticEdge(callerEnv, receiverEnv map[string]string) []string {
	reasons := make([]string, 0)

	if callerEnv["MTLS_ENFORCE"] != "true" {
		reasons = append(reasons, "caller_env_invalid:MTLS_ENFORCE")
	}
	if callerEnv["MTLS_CERT_FILE"] != ExpectedCertFile {
		reasons = append(reasons, "caller_env_invalid:MTLS_CERT_FILE")
	}
	if callerEnv["MTLS_KEY_FILE"] != ExpectedKeyFile {
		reasons = append(reasons, "caller_env_invalid:MTLS_KEY_FILE")
	}
	if callerEnv["MTLS_CA_FILE"] != ExpectedCAFile {
		reasons = append(reasons, "caller_env_invalid:MTLS_CA_FILE")
	}

	if !isValidReceiverURL(callerEnv["NEWS_CREATOR_HOST"]) {
		reasons = append(reasons, "caller_env_invalid:NEWS_CREATOR_HOST")
	}

	effectiveSNI := callerEnv["NEWS_CREATOR_MTLS_SERVER_NAME"]
	if effectiveSNI == "" {
		effectiveSNI = DefaultCallerSNI
	}
	if !commaListContainsExact(receiverEnv["CERT_SANS"], effectiveSNI) {
		reasons = append(reasons, "sni_not_in_receiver_sans:NEWS_CREATOR_MTLS_SERVER_NAME,CERT_SANS")
	}

	if receiverEnv["INBOUND_MTLS"] != "true" {
		reasons = append(reasons, "receiver_env_invalid:INBOUND_MTLS")
	}
	if receiverEnv["PEER_IDENTITY_STRICT"] != "true" {
		reasons = append(reasons, "receiver_env_invalid:PEER_IDENTITY_STRICT")
	}

	callerSubject := callerEnv["CERT_SUBJECT"]
	if callerSubject != CallerService {
		reasons = append(reasons, "caller_env_invalid:CERT_SUBJECT")
	}
	reasons = appendForbiddenKeyReasons(reasons, callerEnv, callerForbiddenFileKeys, "caller_env_invalid:")
	reasons = appendForbiddenKeyReasons(reasons, receiverEnv, receiverForbiddenFileKeys, "receiver_env_invalid:")
	if !commaListContainsExact(receiverEnv["MTLS_ALLOWED_PEERS"], callerSubject) {
		reasons = append(reasons, "caller_not_in_allowed_peers:CERT_SUBJECT,MTLS_ALLOWED_PEERS")
	}

	return reasons
}

func appendForbiddenKeyReasons(reasons []string, env map[string]string, keys []string, prefix string) []string {
	for _, k := range keys {
		if _, ok := env[k]; ok {
			reasons = append(reasons, prefix+k)
		}
	}
	return reasons
}

func isValidReceiverURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Hostname() != ReceiverService || u.Port() != "9443" {
		return false
	}
	return u.Path == ""
}

func commaListContainsExact(listStr, target string) bool {
	for _, part := range strings.Split(listStr, ",") {
		if strings.TrimSpace(part) == target {
			return true
		}
	}
	return false
}

var unsetVarRegex = regexp.MustCompile(`The "([A-Za-z_][A-Za-z0-9_]*)" variable is not set`)

// ExtractComposeUnsetVariables extracts variable names from `docker compose config` stderr
// warnings matching `The "<VAR>" variable is not set` (handling escaped quotes).
func ExtractComposeUnsetVariables(stderr string) []string {
	cleaned := strings.ReplaceAll(stderr, `\`, "")
	matches := unsetVarRegex.FindAllStringSubmatch(cleaned, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

type composeConfigDoc struct {
	Services map[string]struct {
		Environment map[string]string `json:"environment"`
	} `json:"services"`
}

// ParseComposeServiceEnvs extracts the rendered environment maps for caller and receiver
// from `docker compose config --format json` output in memory.
func ParseComposeServiceEnvs(rawJSON []byte) (callerEnv, receiverEnv map[string]string, err error) {
	var doc composeConfigDoc
	if err := json.Unmarshal(rawJSON, &doc); err != nil {
		return nil, nil, fmt.Errorf("invalid compose config json")
	}
	callerSvc, ok := doc.Services[CallerService]
	if !ok {
		return nil, nil, fmt.Errorf("missing service %s", CallerService)
	}
	receiverSvc, ok := doc.Services[ReceiverService]
	if !ok {
		return nil, nil, fmt.Errorf("missing service %s", ReceiverService)
	}
	return callerSvc.Environment, receiverSvc.Environment, nil
}

func renderComposeServiceEnvs(ctx context.Context, runner compose.Executor, composeFile, envFile string) (callerEnv, receiverEnv map[string]string, reasons []string) {
	var stdoutBuf, stderrBuf bytes.Buffer
	runErr := runner.RunWithPipes(
		ctx,
		"docker",
		[]string{"compose", "-f", composeFile, "--env-file", envFile, "-p", ComposeProjectName, "config", "--format", "json"},
		&stdoutBuf,
		&stderrBuf,
	)
	unsetVars := ExtractComposeUnsetVariables(stderrBuf.String())
	if len(unsetVars) > 0 {
		for _, v := range unsetVars {
			reasons = append(reasons, "compose_unset_variable:"+v)
		}
		return nil, nil, reasons
	}
	if runErr != nil {
		return nil, nil, []string{"compose_config_failed"}
	}
	cEnv, rEnv, err := ParseComposeServiceEnvs(stdoutBuf.Bytes())
	if err != nil {
		return nil, nil, []string{"compose_config_failed"}
	}
	return cEnv, rEnv, nil
}
