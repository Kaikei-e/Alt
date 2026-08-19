package pki

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.step.sm/crypto/jose"
)

func compactJWEWithHeader(header map[string]any) string {
	raw, err := json.Marshal(header)
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw) + ".aaaa.bbbb.cccc.dddd"
}

func encryptJWKWithCount(t *testing.T, jwk *jose.JSONWebKey, password []byte, p2c int) string {
	t.Helper()
	raw, err := json.Marshal(jwk)
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, jose.PBKDF2SaltSize)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	encrypter, err := jose.NewEncrypter(jose.DefaultEncAlgorithm, jose.Recipient{
		Algorithm:  jose.PBES2_HS256_A128KW,
		Key:        password,
		PBES2Count: p2c,
		PBES2Salt:  salt,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwe, err := encrypter.Encrypt(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := jwe.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDecryptProvisionerJWK_RejectsNonStepCAP2CBeforeDecrypt(t *testing.T) {
	jwk, err := jose.GenerateJWK("EC", "P-256", "ES256", "sig", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	password := []byte("allowlist-p2c")
	compact := encryptJWKWithCount(t, jwk, password, 1000)
	start := time.Now()
	_, err = decryptProvisionerJWK(compact, password)
	if err == nil || !strings.Contains(err.Error(), "p2c") {
		t.Fatalf("want p2c reject, got %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("header allowlist must reject before PBKDF2, took %s", time.Since(start))
	}
}

func TestDecryptProvisionerJWK_RejectsHugeP2CBeforeDecrypt(t *testing.T) {
	compact := compactJWEWithHeader(map[string]any{
		"alg": "PBES2-HS256+A128KW",
		"enc": "A256GCM",
		"p2c": 999_999_999,
		"p2s": "c2FsdA",
	})
	start := time.Now()
	_, err := decryptProvisionerJWK(compact, []byte("pw"))
	if err == nil || !strings.Contains(err.Error(), "p2c") {
		t.Fatalf("want p2c reject, got %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("huge p2c must not reach PBKDF2, took %s", time.Since(start))
	}
}

func TestDecryptProvisionerJWK_RejectsZipAndUnknownHeaders(t *testing.T) {
	zipCompact := compactJWEWithHeader(map[string]any{
		"alg": "PBES2-HS256+A128KW",
		"enc": "A256GCM",
		"p2c": 600000,
		"p2s": "c2FsdA",
		"zip": "DEF",
	})
	_, err := decryptProvisionerJWK(zipCompact, []byte("pw"))
	if err == nil || !strings.Contains(err.Error(), "zip") {
		t.Fatalf("want zip reject, got %v", err)
	}

	unknown := compactJWEWithHeader(map[string]any{
		"alg": "PBES2-HS256+A128KW",
		"enc": "A256GCM",
		"p2c": 600000,
		"p2s": "c2FsdA",
		"foo": "bar",
	})
	_, err = decryptProvisionerJWK(unknown, []byte("pw"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "header") {
		t.Fatalf("want unknown header reject, got %v", err)
	}
}

func TestDecryptProvisionerJWK_RejectsWrongAlgEncAndJSONSerialization(t *testing.T) {
	wrongAlg := compactJWEWithHeader(map[string]any{
		"alg": "PBES2-HS512+A256KW",
		"enc": "A256GCM",
		"p2c": 600000,
		"p2s": "c2FsdA",
	})
	_, err := decryptProvisionerJWK(wrongAlg, []byte("pw"))
	if err == nil || !strings.Contains(err.Error(), "alg") {
		t.Fatalf("want alg reject, got %v", err)
	}

	wrongEnc := compactJWEWithHeader(map[string]any{
		"alg": "PBES2-HS256+A128KW",
		"enc": "A128GCM",
		"p2c": 600000,
		"p2s": "c2FsdA",
	})
	_, err = decryptProvisionerJWK(wrongEnc, []byte("pw"))
	if err == nil || !strings.Contains(err.Error(), "enc") {
		t.Fatalf("want enc reject, got %v", err)
	}

	_, err = decryptProvisionerJWK(`{"protected":"e30","encrypted_key":"aa","iv":"aa","ciphertext":"aa","tag":"aa"}`, []byte("pw"))
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("want compact-only reject, got %v", err)
	}
}

func TestDecryptProvisionerJWK_RejectsOversizedCompact(t *testing.T) {
	compact := compactJWEWithHeader(map[string]any{
		"alg": "PBES2-HS256+A128KW",
		"enc": "A256GCM",
		"p2c": 600000,
		"p2s": "c2FsdA",
	}) + strings.Repeat("A", (8<<10))
	_, err := decryptProvisionerJWK(compact, []byte("pw"))
	if err == nil || !strings.Contains(err.Error(), "size cap") {
		t.Fatalf("want compact size cap, got %v", err)
	}
}

func TestDecryptProvisionerJWK_AcceptsExactStepCAHeader(t *testing.T) {
	jwk, err := jose.GenerateJWK("EC", "P-256", "ES256", "sig", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	password := []byte("step-ca-p2c")
	compact := encryptJWKWithCount(t, jwk, password, 600000)
	got, err := decryptProvisionerJWK(compact, password)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Key == nil {
		t.Fatal("expected decrypted JWK")
	}
}

func TestDecryptProvisionerJWK_RejectsOversizedPlaintext(t *testing.T) {
	password := []byte("big-jwk")
	salt := make([]byte, jose.PBKDF2SaltSize)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	encrypter, err := jose.NewEncrypter(jose.DefaultEncAlgorithm, jose.Recipient{
		Algorithm:  jose.PBES2_HS256_A128KW,
		Key:        password,
		PBES2Count: 600000,
		PBES2Salt:  salt,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwe, err := encrypter.Encrypt([]byte(strings.Repeat("A", (8<<10)+1)))
	if err != nil {
		t.Fatal(err)
	}
	compact, err := jwe.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	_, err = decryptProvisionerJWK(compact, password)
	if err == nil || !strings.Contains(err.Error(), "size cap") {
		t.Fatalf("want plaintext size cap, got %v", err)
	}
}
