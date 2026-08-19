package pki

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.step.sm/crypto/jose"
)

const (
	stepCAPBES2P2C     = 600000
	stepCAPBES2Alg     = "PBES2-HS256+A128KW"
	stepCAPBES2Enc     = "A256GCM"
	maxJWECompactBytes = 8 << 10
	maxJWEHeaderBytes  = 1024
	maxJWKJSONBytes    = 8 << 10
)

var allowedJWEHeaderKeys = map[string]struct{}{
	"alg": {},
	"enc": {},
	"p2c": {},
	"p2s": {},
	"kid": {},
	"typ": {},
	"cty": {},
}

func inspectProvisionerJWE(encryptedKey string) error {
	if len(encryptedKey) > maxJWECompactBytes {
		return fmt.Errorf("pki: provisioner JWE exceeded size cap")
	}
	parts := strings.Split(encryptedKey, ".")
	if len(parts) != 5 {
		return fmt.Errorf("pki: malformed provisioner jwe")
	}
	raw, err := decodeJWEProtectedHeader(parts[0])
	if err != nil {
		return fmt.Errorf("pki: malformed provisioner jwe header")
	}
	if len(raw) > maxJWEHeaderBytes {
		return fmt.Errorf("pki: provisioner JWE header exceeded size cap")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var header map[string]any
	if err := dec.Decode(&header); err != nil {
		return fmt.Errorf("pki: malformed provisioner jwe header")
	}
	if dec.More() {
		return fmt.Errorf("pki: malformed provisioner jwe header")
	}
	if _, ok := header["zip"]; ok {
		return fmt.Errorf("pki: unexpected provisioner JWE zip")
	}
	for key := range header {
		if _, ok := allowedJWEHeaderKeys[key]; !ok {
			return fmt.Errorf("pki: unexpected provisioner JWE header %q", key)
		}
	}
	alg, ok := headerString(header["alg"])
	if !ok || alg != stepCAPBES2Alg {
		return fmt.Errorf("pki: unexpected provisioner JWE alg")
	}
	enc, ok := headerString(header["enc"])
	if !ok || enc != stepCAPBES2Enc {
		return fmt.Errorf("pki: unexpected provisioner JWE enc")
	}
	p2c, ok := headerUint(header["p2c"])
	if !ok || p2c != stepCAPBES2P2C {
		return fmt.Errorf("pki: unexpected provisioner JWE p2c")
	}
	if p2s, ok := headerString(header["p2s"]); !ok || p2s == "" {
		return fmt.Errorf("pki: unexpected provisioner JWE p2s")
	}
	return nil
}

func decodeJWEProtectedHeader(seg string) ([]byte, error) {
	if raw, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
		return raw, nil
	}
	return base64.URLEncoding.DecodeString(seg)
}

func headerString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok && s != ""
}

func headerUint(v any) (uint64, bool) {
	switch n := v.(type) {
	case json.Number:
		u, err := strconv.ParseUint(n.String(), 10, 64)
		return u, err == nil
	case float64:
		if n < 0 || n != float64(uint64(n)) {
			return 0, false
		}
		return uint64(n), true
	default:
		return 0, false
	}
}

func decryptProvisionerJWK(encryptedKey string, password []byte) (*jose.JSONWebKey, error) {
	if err := inspectProvisionerJWE(encryptedKey); err != nil {
		return nil, err
	}
	enc, err := jose.ParseEncrypted(encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("pki: parse provisioner jwe: %w", err)
	}
	data, err := enc.Decrypt(password)
	if err != nil {
		return nil, fmt.Errorf("pki: decrypt provisioner jwk: %w", err)
	}
	if len(data) > maxJWKJSONBytes {
		return nil, fmt.Errorf("pki: decrypted provisioner JWK exceeded size cap")
	}
	jwk := new(jose.JSONWebKey)
	if err := json.Unmarshal(data, jwk); err != nil {
		return nil, fmt.Errorf("pki: unmarshal provisioner jwk: %w", err)
	}
	return jwk, nil
}
