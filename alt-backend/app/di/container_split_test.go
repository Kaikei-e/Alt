package di

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"alt/config"
)

func fieldNames(t *testing.T, v any) map[string]bool {
	t.Helper()
	rt := reflect.TypeOf(v)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		t.Fatalf("expected a struct, got %s", rt.Kind())
	}
	names := make(map[string]bool, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		names[rt.Field(i).Name] = true
	}
	return names
}

// The three composition roots exist so that "this binary does not build X" is
// enforced by the compiler rather than by a nil field nobody checks. A field
// that is present but nil is exactly the ADR-000928 shape: a handler reaching
// for it cannot tell "DI forgot to wire this" from "deliberately disabled".
// Absent field, absent surface, compile error — that is the whole point of
// splitting the container, so it is pinned here.
//
// cmd/datahub's half of this table lives in di/datahub/container_test.go. That
// root became a package of its own in ADR-000954 Wave 3 batch 6 — it is the
// only one that constructs an alt_db repository, and a package is Go's unit of
// linkage — so a test in this package cannot import it without a cycle.
func TestComponentStructs_OmitWhatTheirBinaryDoesNotBuild(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		absent  []string
		present []string
	}{
		{
			name:  "backend",
			value: ApplicationComponents{},
			absent: []string{
				// Moved to cmd/datahub with DataHubService (ADR-000954 D6/D7).
				"KratosClient", "EventPublisher", "RecapArticlesUsecase",
				"FetchRecentArticlesUsecase", "CreateTagSetVersionUsecase",
				// Never read by any handler (plan R11): copying them into three
				// roots would have tripled the dead wiring.
				"ConfigPort", "RateLimiterPort", "ErrorHandlerPort",
				"AppendKnowledgeEventUsecase",
				// ADR-000954 Wave 3 batch 6. The Tag Trail's paged read and
				// RecallRailUsecase's article fallback were the last two
				// capabilities holding a pool open here; both are procedures
				// now, so the backend has no database handle and no gateway
				// wrapping one. InternalArticleGateway went with it — it
				// existed here only for that fallback — and it moved to
				// dataplane/ with the rest of the provider-side gateways.
				"AltDBRepository", "InternalArticleGateway",
			},
			present: []string{
				"SovereignClient", "AdminMonitor", "RecallRailUsecase",
				"CreateSummaryVersionUsecase", "ImageProxyUsecase", "CSRFTokenUsecase",
			},
		},
		{
			name:  "harvester",
			value: HarvesterComponents{},
			absent: []string{
				// None of the eight scheduled jobs search, publish events, or
				// answer HTTP, so none of this may be constructed here.
				"SearchIndexerDriver", "MQHubClient", "EventPublisher", "KratosClient",
				"CSRFTokenUsecase", "RagConnectClient", "AdminMonitor",
				"InternalArticleGateway", "RecapArticlesUsecase",
				// ADR-000954 Wave 3 batch 5: the tag cloud was the harvester's
				// last direct read, so it has no database handle at all. This
				// is the first of the three binaries to reach Wave 3's exit
				// condition, and the absence is asserted rather than merely
				// achieved — a job that reaches for a pool must not compile.
				"AltDBRepository",
			},
			present: []string{
				"ScrapingDomainUsecase", "RagIntegration",
				"SovereignClient", "ImageProxyUsecase", "FetchArticleGateway",
				"FetchTagCloudUsecase",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := fieldNames(t, tt.value)
			for _, f := range tt.absent {
				if names[f] {
					t.Errorf("%s components must not carry field %s", tt.name, f)
				}
			}
			for _, f := range tt.present {
				if !names[f] {
					t.Errorf("%s components must carry field %s", tt.name, f)
				}
			}
		})
	}
}

func splitTestConfig() *config.Config {
	return &config.Config{
		AppEnv: "development",
		Server: config.ServerConfig{
			Port: 9000, ConnectPort: 9101,
			ReadTimeout: 300 * time.Second, WriteTimeout: 300 * time.Second, IdleTimeout: 120 * time.Second,
		},
		RateLimit:     config.RateLimitConfig{ExternalAPIInterval: 10 * time.Second, ExternalAPIBurst: 3, FeedFetchLimit: 100},
		SearchIndexer: config.SearchIndexerConfig{ConnectURL: "http://search-indexer:9301"},
		MQHub:         config.MQHubConfig{Enabled: false, ConnectURL: "http://mq-hub:9500"},
	}
}

// newFeedModule used to return DeleteFeedLinkUsecase: nil and rely on the one
// composition root to patch it afterwards. With three roots, forgetting the
// patch in one of them still compiles and only fails as a nil dereference in
// the RSS DeleteFeedLink handler, so the dependency is passed in instead.
func TestNewFeedModule_WiresDeleteFeedLinkUsecase(t *testing.T) {
	setDataHubClientEnv(t)

	infra := newInfraModule(splitTestConfig())
	sub := newSubscriptionModule(infra)

	feed := newFeedModule(infra, sub)

	if feed.DeleteFeedLinkUsecase == nil {
		t.Fatal("newFeedModule must wire DeleteFeedLinkUsecase itself, not leave it for the composition root")
	}
	if feed.DeleteFeedLinkUsecase != sub.DeleteFeedLinkUsecase {
		t.Error("DeleteFeedLinkUsecase must be the subscription module's instance")
	}
}

// setDataHubClientEnv gives newInfraModule / NewHarvesterComponents a valid
// alt-data-hub client configuration.
//
// It is required rather than optional: cmd/backend and cmd/harvester have no
// database of their own for the capabilities ADR-000954 Wave 3 moved, so
// config.LoadDataHubClientConfig fails closed and the composition root panics
// on it (CLAUDE.md rule 9). The certificate is a throwaway self-signed leaf
// used as its own trust root — these tests are about wiring, and the chain is
// tlsutil's concern.
func setDataHubClientEnv(t *testing.T) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "alt-backend"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"alt-backend"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create test certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal test key: %v", err)
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "svc-cert.pem")
	keyPath := filepath.Join(dir, "svc-key.pem")
	caPath := filepath.Join(dir, "ca-bundle.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	for path, content := range map[string][]byte{
		certPath: certPEM,
		caPath:   certPEM,
		keyPath:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	t.Setenv("DATA_HUB_MTLS_URL", "https://alt-data-hub:9443")
	t.Setenv("DATA_HUB_SERVER_NAME", "alt-data-hub")
	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)
}
