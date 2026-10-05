package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// repoBaseComposeFile resolves compose/base.yaml from this test file's own
// location (runtime.Caller), independent of `go test`'s working directory,
// so the test is not coupled to findBaseComposeFile's cwd-walk logic (that
// logic is exercised separately by TestFindBaseComposeFile_LocatesRealFile).
func repoBaseComposeFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// this file: <repo>/altctl/internal/setup/secrets_test.go
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	path := filepath.Join(repoRoot, "compose", "base.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("resolved compose/base.yaml does not exist at %s: %v", path, err)
	}
	return path
}

// requiredSecretsFromBaseYAML is required's ground truth: it re-derives the
// secret filename list straight from compose/base.yaml's secrets: block,
// independent of DefaultSecretSpecs/knownSecretMeta, so this test can't pass
// just because the production code and the test list drifted together.
func requiredSecretsFromBaseYAML(t *testing.T) []string {
	t.Helper()
	names, err := secretNamesFromComposeFile(repoBaseComposeFile(t))
	if err != nil {
		t.Fatalf("secretNamesFromComposeFile: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("compose/base.yaml secrets: block parsed to zero names")
	}
	return names
}

// TestKnownSecretMeta_CoversEveryComposeSecret is the drift gate between
// compose/base.yaml and the provisioning table. A secret declared in compose
// with no class is the 2026-10-02 failure: nothing provisioned it, and the
// deploy only found out when the container would not start.
func TestKnownSecretMeta_CoversEveryComposeSecret(t *testing.T) {
	var unclassified []string
	for _, name := range requiredSecretsFromBaseYAML(t) {
		if _, ok := metaFor(name); !ok {
			unclassified = append(unclassified, name)
		}
	}
	if len(unclassified) > 0 {
		t.Errorf("compose/base.yaml declares secrets with no provisioning class in knownSecretMeta: %v", unclassified)
	}
}

// TestDefaultSecretSpecs_ContainsAllRequired is the regression test for the
// missing-8-secrets bug: DefaultSecretSpecs must contain every secret
// compose/base.yaml declares, each provisioned by the right class.
func TestDefaultSecretSpecs_ContainsAllRequired(t *testing.T) {
	required := requiredSecretsFromBaseYAML(t)

	specs, err := DefaultSecretSpecs()
	if err != nil {
		t.Fatalf("DefaultSecretSpecs: %v", err)
	}
	specMap := make(map[string]SecretSpec, len(specs))
	for _, s := range specs {
		specMap[s.Filename] = s
	}

	operatorProvided := map[string]bool{
		"hugging_face_token.txt":      true,
		"inoreader_client_id.txt":     true,
		"inoreader_client_secret.txt": true,
		"k6_api_token.txt":            true,
		"pushover_user_key.txt":       true,
		"pushover_token.txt":          true,
		// A P-256 keypair whose halves must match; random bytes in either
		// file would invalidate every browser subscription.
		"vapid_private_key.txt": true,
		"vapid_public_key.txt":  true,
	}

	for _, name := range required {
		spec, ok := specMap[name]
		if !ok {
			t.Errorf("DefaultSecretSpecs missing secret declared in compose/base.yaml: %s", name)
			continue
		}
		want := SecretClassRandom
		switch {
		case operatorProvided[name]:
			want = SecretClassOperatorProvided
		case strings.HasPrefix(name, "pki-agent-") && strings.HasSuffix(name, "-jwk.txt"):
			want = SecretClassPKIProvisioner
		}
		if spec.Class != want {
			t.Errorf("%s: Class = %q, want %q", name, spec.Class, want)
		}
	}
}

// TestDefaultSecretSpecs_MqhubAuthTokenIsRandom pins the secret the
// 2026-10-02 remediation added to compose without a provisioning entry.
func TestDefaultSecretSpecs_MqhubAuthTokenIsRandom(t *testing.T) {
	specs, err := DefaultSecretSpecs()
	if err != nil {
		t.Fatalf("DefaultSecretSpecs: %v", err)
	}
	for _, s := range specs {
		if s.Filename == "mqhub_auth_token.txt" {
			if s.Class != SecretClassRandom || s.Length != 32 {
				t.Errorf("mqhub_auth_token.txt = %+v, want random with 32 bytes", s)
			}
			return
		}
	}
	t.Fatal("mqhub_auth_token.txt not found in DefaultSecretSpecs")
}

// TestPKIBootstrapScript_ProvisionsEveryComposeSubject guards the hand-off a
// pki-provisioner secret relies on: altctl tells the operator to run the
// bootstrap script, so that script must know every subject compose mounts a
// provisioner password for, or the instruction silently does nothing.
func TestPKIBootstrapScript_ProvisionsEveryComposeSubject(t *testing.T) {
	repoRoot := filepath.Dir(filepath.Dir(repoBaseComposeFile(t)))
	script, err := os.ReadFile(filepath.Join(repoRoot, PKIBootstrapScript))
	if err != nil {
		t.Fatalf("reading %s: %v", PKIBootstrapScript, err)
	}

	subjects := map[string]bool{}
	inArray := false
	for _, line := range strings.Split(string(script), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "SUBJECTS=(":
			inArray = true
		case inArray && trimmed == ")":
			inArray = false
		case inArray && trimmed != "" && !strings.HasPrefix(trimmed, "#"):
			subjects[trimmed] = true
		}
	}
	if len(subjects) == 0 {
		t.Fatalf("no SUBJECTS=( ... ) array found in %s", PKIBootstrapScript)
	}

	for _, name := range requiredSecretsFromBaseYAML(t) {
		meta, ok := metaFor(name)
		if !ok || meta.Class != SecretClassPKIProvisioner {
			continue
		}
		subject := strings.TrimSuffix(strings.TrimPrefix(name, "pki-agent-"), "-jwk.txt")
		if !subjects[subject] {
			t.Errorf("compose/base.yaml mounts %s, but %s has no %q subject to create it", name, PKIBootstrapScript, subject)
		}
	}
}

// TestDefaultSecretSpecs_ExactlyMatchesBaseYAML asserts there's no drift in
// either direction: no secret base.yaml requires is missing, and no dead
// secret (removed from base.yaml, e.g. by the mTLS migration ADR-000743
// that deleted service_secret.txt) lingers in the generated list.
func TestDefaultSecretSpecs_ExactlyMatchesBaseYAML(t *testing.T) {
	required := requiredSecretsFromBaseYAML(t)
	requiredSet := make(map[string]bool, len(required))
	for _, n := range required {
		requiredSet[n] = true
	}

	specs, err := DefaultSecretSpecs()
	if err != nil {
		t.Fatalf("DefaultSecretSpecs: %v", err)
	}
	gotSet := make(map[string]bool, len(specs))
	for _, s := range specs {
		gotSet[s.Filename] = true
	}

	for name := range requiredSet {
		if !gotSet[name] {
			t.Errorf("missing from DefaultSecretSpecs: %s", name)
		}
	}
	for name := range gotSet {
		if !requiredSet[name] {
			t.Errorf("DefaultSecretSpecs has a secret not declared in compose/base.yaml (dead entry?): %s", name)
		}
	}
}

// TestDefaultSecretSpecs_NoDeadSecrets pins down the specific dead entries
// the bug report named: three phantom per-service DB passwords nothing
// reads, a dead shared-auth secret, and service_secret.txt (removed by the
// mTLS migration, ADR-000743).
func TestDefaultSecretSpecs_NoDeadSecrets(t *testing.T) {
	dead := []string{
		"pre_processor_db_password.txt",
		"tag_generator_db_password.txt",
		"search_indexer_db_password.txt",
		"auth_shared_secret.txt",
		"service_secret.txt",
	}

	specs, err := DefaultSecretSpecs()
	if err != nil {
		t.Fatalf("DefaultSecretSpecs: %v", err)
	}
	specMap := make(map[string]bool, len(specs))
	for _, s := range specs {
		specMap[s.Filename] = true
	}

	for _, name := range dead {
		if specMap[name] {
			t.Errorf("dead secret %s should not be in DefaultSecretSpecs", name)
		}
	}
}

// TestDefaultSecretSpecs_KratosCipherSecretTruncatedTo32 guards a specific
// generation-strategy requirement Kratos enforces: the cipher secret must
// be exactly 32 characters.
func TestDefaultSecretSpecs_KratosCipherSecretTruncatedTo32(t *testing.T) {
	specs, err := DefaultSecretSpecs()
	if err != nil {
		t.Fatalf("DefaultSecretSpecs: %v", err)
	}
	for _, s := range specs {
		if s.Filename == "kratos_cipher_secret.txt" {
			if s.Truncate != 32 {
				t.Errorf("kratos_cipher_secret.txt Truncate = %d, want 32", s.Truncate)
			}
			return
		}
	}
	t.Fatal("kratos_cipher_secret.txt not found in DefaultSecretSpecs")
}

func TestDefaultSecretSpecs_OperatorProvidedSecretsNotGenerated(t *testing.T) {
	specs, err := DefaultSecretSpecs()
	if err != nil {
		t.Fatalf("DefaultSecretSpecs: %v", err)
	}

	optional := []string{
		"hugging_face_token.txt",
		"inoreader_client_id.txt",
		"inoreader_client_secret.txt",
	}

	specMap := make(map[string]SecretSpec)
	for _, s := range specs {
		specMap[s.Filename] = s
	}

	for _, name := range optional {
		spec, ok := specMap[name]
		if !ok {
			t.Errorf("missing optional secret spec: %s", name)
			continue
		}
		if spec.Class != SecretClassOperatorProvided {
			t.Errorf("optional secret %s should be operator-provided, got %q", name, spec.Class)
		}
	}
}

// TestFindBaseComposeFile_LocatesRealFile exercises the production
// cwd-walk lookup (as opposed to repoBaseComposeFile's runtime.Caller
// shortcut used elsewhere in this file) to make sure DefaultSecretSpecs'
// actual runtime path-finding works from this package's test cwd.
func TestFindBaseComposeFile_LocatesRealFile(t *testing.T) {
	path, err := findBaseComposeFile()
	if err != nil {
		t.Fatalf("findBaseComposeFile: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("findBaseComposeFile returned %s, but it doesn't exist: %v", path, err)
	}
	if filepath.Base(path) != "base.yaml" {
		t.Errorf("findBaseComposeFile returned %s, want a base.yaml path", path)
	}
}

// TestDeriveSecretSpecs_UnknownSecretFailsFast: a secret compose declares
// that altctl cannot classify must stop `altctl init`, not get random bytes.
// Random bytes are wrong for two of the three classes — a pki-agent
// provisioner password nobody registered with step-ca, or half of a keypair —
// and the service only discovers that at enrollment or signing time.
func TestDeriveSecretSpecs_UnknownSecretFailsFast(t *testing.T) {
	dir := t.TempDir()
	fakeBase := filepath.Join(dir, "base.yaml")
	content := `
secrets:
  totally_new_secret_nobody_documented_yet:
    file: ../secrets/totally_new_secret_nobody_documented_yet.txt
  postgres_password:
    file: ../secrets/postgres_password.txt
`
	if err := os.WriteFile(fakeBase, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	specs, err := DeriveSecretSpecs(fakeBase)
	if err == nil {
		t.Fatalf("expected an error for an unclassified secret, got %d specs", len(specs))
	}
	if !strings.Contains(err.Error(), "totally_new_secret_nobody_documented_yet.txt") {
		t.Errorf("error should name the unclassified secret, got: %v", err)
	}
}

func TestDeriveSecretSpecs_PKIAgentSecretIsPKIProvisioner(t *testing.T) {
	dir := t.TempDir()
	fakeBase := filepath.Join(dir, "base.yaml")
	content := `
secrets:
  pki-agent-some-new-service-jwk:
    file: ../secrets/pki-agent-some-new-service-jwk.txt
`
	if err := os.WriteFile(fakeBase, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	specs, err := DeriveSecretSpecs(fakeBase)
	if err != nil {
		t.Fatalf("DeriveSecretSpecs: %v", err)
	}
	if len(specs) != 1 || specs[0].Class != SecretClassPKIProvisioner {
		t.Fatalf("specs = %+v, want one pki-provisioner spec", specs)
	}
}

// TestDefaultSecretSpecs_ErrorsWhenBaseComposeFileNotFound is the C2
// regression test: DefaultSecretSpecs used to silently fall back to a
// hardcoded 3-secret list (fallbackSecretSpecs) whenever compose/base.yaml
// couldn't be located, and cmd/init.go both generated AND validated against
// that same shrunken list -- so `altctl init` reported success having
// created only 3 of the 24 required secret files. The fallback is gone:
// DefaultSecretSpecs must now return a loud error instead of a degraded
// list when it's invoked somewhere with no compose/base.yaml above it in
// the directory tree (e.g. altctl run from outside an Alt checkout).
func TestDefaultSecretSpecs_ErrorsWhenBaseComposeFileNotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	specs, err := DefaultSecretSpecs()
	if err == nil {
		t.Fatalf("expected an error when compose/base.yaml cannot be found, got %d specs", len(specs))
	}
	if specs != nil {
		t.Errorf("expected nil specs alongside the error, got %v", specs)
	}
}

// TestDeriveSecretSpecs_MissingSecretsBlock exercises the error path: a
// compose file with no top-level secrets: block should fail loudly rather
// than silently return an empty spec list (which would make `altctl init`
// generate zero secrets without any indication why).
func TestDeriveSecretSpecs_MissingSecretsBlock(t *testing.T) {
	dir := t.TempDir()
	fakeBase := filepath.Join(dir, "base.yaml")
	if err := os.WriteFile(fakeBase, []byte("name: alt\nnetworks:\n  alt-network:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := DeriveSecretSpecs(fakeBase); err == nil {
		t.Error("expected an error for a compose file with no secrets: block")
	}
}

func TestGenerateSecrets_CreatesFiles(t *testing.T) {
	dir := t.TempDir()

	specs := []SecretSpec{
		{Filename: "test_password.txt", Class: SecretClassRandom, Length: 32},
		{Filename: "test_token.txt", Class: SecretClassOperatorProvided},
	}

	result, err := GenerateSecrets(dir, specs, false)
	if err != nil {
		t.Fatalf("GenerateSecrets failed: %v", err)
	}

	if len(result.Created) != 2 {
		t.Errorf("expected 2 created, got %d", len(result.Created))
	}

	// Auto-generated file should have content
	content, err := os.ReadFile(filepath.Join(dir, "test_password.txt"))
	if err != nil {
		t.Fatalf("reading auto-generated secret: %v", err)
	}
	if len(content) == 0 {
		t.Error("auto-generated secret should not be empty")
	}

	// Optional file should be empty
	content, err = os.ReadFile(filepath.Join(dir, "test_token.txt"))
	if err != nil {
		t.Fatalf("reading optional secret: %v", err)
	}
	if len(content) != 0 {
		t.Error("optional secret should be empty")
	}
}

func TestGenerateSecrets_Idempotent(t *testing.T) {
	dir := t.TempDir()

	specs := []SecretSpec{
		{Filename: "test_password.txt", Class: SecretClassRandom, Length: 32},
	}

	// First run
	_, err := GenerateSecrets(dir, specs, false)
	if err != nil {
		t.Fatalf("first GenerateSecrets failed: %v", err)
	}

	// Read the generated value
	original, _ := os.ReadFile(filepath.Join(dir, "test_password.txt"))

	// Second run - should skip
	result, err := GenerateSecrets(dir, specs, false)
	if err != nil {
		t.Fatalf("second GenerateSecrets failed: %v", err)
	}

	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(result.Skipped))
	}

	// Value should be unchanged
	after, _ := os.ReadFile(filepath.Join(dir, "test_password.txt"))
	if string(original) != string(after) {
		t.Error("idempotent run should not change existing secret")
	}
}

func TestGenerateSecrets_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()

	specs := []SecretSpec{
		{Filename: "test_password.txt", Class: SecretClassRandom, Length: 32},
	}

	// First run
	_, _ = GenerateSecrets(dir, specs, false)
	original, _ := os.ReadFile(filepath.Join(dir, "test_password.txt"))

	// Force run
	result, err := GenerateSecrets(dir, specs, true)
	if err != nil {
		t.Fatalf("force GenerateSecrets failed: %v", err)
	}

	if len(result.Created) != 1 {
		t.Errorf("expected 1 created (force), got %d", len(result.Created))
	}

	after, _ := os.ReadFile(filepath.Join(dir, "test_password.txt"))
	if string(original) == string(after) {
		t.Error("force should generate a new value")
	}
}

// Compose bind-mounts a `file:`-style secret into the container with the
// host file's mode intact, and Alt's images run as nonroot (UID 65532): an
// owner-only secret reads back as empty there and the service treats it as
// "not configured" rather than failing (on koko-b that turned a 0600
// meili_master_key into MEILISEARCH_API_KEY=placeholder and 403s). The
// canonical secrets dir is also shared with a second user — the alt-prod CI
// runner stages from it over the altcfg group — so owner-only files break
// the deploy's staging step as well.
//
// The umask is forced here because os.WriteFile's perm argument is masked by
// it: without an explicit chmod, an operator running altctl under umask 077
// gets 0600 files and neither of the two readers above works.
func TestGenerateSecrets_ModeStaysReadableUnderRestrictiveUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(old) })

	dir := t.TempDir()
	specs := []SecretSpec{
		{Filename: "test_password.txt", Class: SecretClassRandom, Length: 32},
		{Filename: "test_token.txt", Class: SecretClassOperatorProvided},
	}

	if _, err := GenerateSecrets(dir, specs, false); err != nil {
		t.Fatalf("GenerateSecrets failed: %v", err)
	}

	for _, spec := range specs {
		info, err := os.Stat(filepath.Join(dir, spec.Filename))
		if err != nil {
			t.Fatalf("stat %s: %v", spec.Filename, err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("%s has mode %#o, want 0644", spec.Filename, got)
		}
	}
}

func TestGenerateRandomSecret_Length(t *testing.T) {
	secret, err := generateRandomSecret(32)
	if err != nil {
		t.Fatalf("generateRandomSecret failed: %v", err)
	}
	// base64 of 32 bytes = 44 chars (with padding)
	if len(secret) == 0 {
		t.Error("secret should not be empty")
	}
}

func TestGenerateRandomSecret_Unique(t *testing.T) {
	s1, _ := generateRandomSecret(32)
	s2, _ := generateRandomSecret(32)
	if s1 == s2 {
		t.Error("two generated secrets should differ")
	}
}

// A pki-provisioner password is only valid once step-ca has a provisioner
// registered with the same bytes. Writing random bytes here is how a deploy
// gets a secret file that exists, mounts, and fails at enrollment.
func TestGenerateSecrets_NeverWritesPKIProvisionerSecret(t *testing.T) {
	dir := t.TempDir()
	specs := []SecretSpec{
		{Filename: "pki-agent-alt-backend-jwk.txt", Class: SecretClassPKIProvisioner},
	}

	for _, force := range []bool{false, true} {
		result, err := GenerateSecrets(dir, specs, force)
		if err != nil {
			t.Fatalf("GenerateSecrets(force=%v): %v", force, err)
		}
		if len(result.Created) != 0 {
			t.Errorf("force=%v: pki-provisioner secret reported as created: %v", force, result.Created)
		}
		if _, err := os.Stat(filepath.Join(dir, "pki-agent-alt-backend-jwk.txt")); !os.IsNotExist(err) {
			t.Fatalf("force=%v: pki-provisioner secret file must not be created, stat err = %v", force, err)
		}
	}
}

// --force regenerates what altctl generated. It must not truncate a value
// issued outside Alt or rotate a provisioner password away from the one
// registered in step-ca.
func TestGenerateSecrets_ForceLeavesNonRandomSecretsAlone(t *testing.T) {
	dir := t.TempDir()
	keep := map[string]string{
		"hugging_face_token.txt":        "hf_operator_value",
		"pki-agent-alt-backend-jwk.txt": "registered-provisioner-password",
	}
	for name, value := range keep {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	specs := []SecretSpec{
		{Filename: "hugging_face_token.txt", Class: SecretClassOperatorProvided},
		{Filename: "pki-agent-alt-backend-jwk.txt", Class: SecretClassPKIProvisioner},
	}

	if _, err := GenerateSecrets(dir, specs, true); err != nil {
		t.Fatalf("GenerateSecrets: %v", err)
	}
	for name, want := range keep {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s was rewritten by --force: got %q, want %q", name, got, want)
		}
	}
}

// The canonical secrets directory is staged by a second user over a shared
// group; an owner-only directory hides the 0644 files inside it.
func TestGenerateSecrets_CreatesGroupReadableDirectory(t *testing.T) {
	old := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(old) })

	dir := filepath.Join(t.TempDir(), "secrets")
	specs := []SecretSpec{{Filename: "test_password.txt", Class: SecretClassRandom, Length: 32}}
	if _, err := GenerateSecrets(dir, specs, false); err != nil {
		t.Fatalf("GenerateSecrets: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o750 {
		t.Errorf("secrets directory mode = %#o, want 0750", got)
	}
}

func TestFindPendingSecrets(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"filled_token.txt":           "value",
		"empty_token.txt":            "",
		"pki-agent-auth-hub-jwk.txt": "registered",
		"generated_password.txt":     "random",
	}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	specs := []SecretSpec{
		{Filename: "filled_token.txt", Class: SecretClassOperatorProvided},
		{Filename: "empty_token.txt", Class: SecretClassOperatorProvided},
		{Filename: "missing_token.txt", Class: SecretClassOperatorProvided},
		{Filename: "pki-agent-auth-hub-jwk.txt", Class: SecretClassPKIProvisioner},
		{Filename: "pki-agent-alt-backend-jwk.txt", Class: SecretClassPKIProvisioner},
		{Filename: "generated_password.txt", Class: SecretClassRandom},
	}

	pending, err := FindPendingSecrets(dir, specs)
	if err != nil {
		t.Fatalf("FindPendingSecrets: %v", err)
	}

	names := func(specs []SecretSpec) string {
		var out []string
		for _, s := range specs {
			out = append(out, s.Filename)
		}
		return strings.Join(out, ",")
	}
	if got := names(pending.OperatorProvided); got != "empty_token.txt,missing_token.txt" {
		t.Errorf("OperatorProvided = %s, want empty_token.txt,missing_token.txt", got)
	}
	if got := names(pending.PKIProvisioner); got != "pki-agent-alt-backend-jwk.txt" {
		t.Errorf("PKIProvisioner = %s, want pki-agent-alt-backend-jwk.txt", got)
	}
}
