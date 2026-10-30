package pki

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

type provisionerJWK struct {
	Kty   string            `json:"kty"`
	Crv   string            `json:"crv"`
	Alg   string            `json:"alg,omitempty"`
	Use   string            `json:"use,omitempty"`
	KeyID string            `json:"kid,omitempty"`
	X     string            `json:"x"`
	Y     string            `json:"y"`
	D     string            `json:"d,omitempty"`
	Key   *ecdsa.PrivateKey `json:"-"`
}

func (j *provisionerJWK) Public() provisionerJWK {
	return provisionerJWK{
		Kty:   j.Kty,
		Crv:   j.Crv,
		Alg:   j.Alg,
		Use:   j.Use,
		KeyID: j.KeyID,
		X:     j.X,
		Y:     j.Y,
	}
}

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

func decryptProvisionerJWK(encryptedKey string, password []byte) (*provisionerJWK, error) {
	if err := inspectProvisionerJWE(encryptedKey); err != nil {
		return nil, err
	}
	data, err := decryptPBES2A256GCM(encryptedKey, password)
	if err != nil {
		return nil, fmt.Errorf("pki: decrypt provisioner jwk: %w", err)
	}
	if len(data) > maxJWKJSONBytes {
		return nil, fmt.Errorf("pki: decrypted provisioner JWK exceeded size cap")
	}
	jwk, err := parseProvisionerECJWK(data)
	if err != nil {
		return nil, fmt.Errorf("pki: unmarshal provisioner jwk: %w", err)
	}
	return jwk, nil
}

func decryptPBES2A256GCM(compact string, password []byte) ([]byte, error) {
	parts := strings.Split(compact, ".")
	if len(parts) != 5 {
		return nil, fmt.Errorf("malformed compact JWE")
	}
	hdrBytes, err := decodeJWEProtectedHeader(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var hdr struct {
		Alg string `json:"alg"`
		P2C int    `json:"p2c"`
		P2S string `json:"p2s"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return nil, fmt.Errorf("unmarshal header: %w", err)
	}
	saltInput, err := decodeJWEProtectedHeader(hdr.P2S)
	if err != nil || len(saltInput) < 8 {
		return nil, fmt.Errorf("invalid p2s salt")
	}
	formattedSalt := make([]byte, 0, len(hdr.Alg)+1+len(saltInput))
	formattedSalt = append(formattedSalt, hdr.Alg...)
	formattedSalt = append(formattedSalt, 0x00)
	formattedSalt = append(formattedSalt, saltInput...)

	kek, err := pbkdf2.Key(sha256.New, string(password), formattedSalt, hdr.P2C, 16)
	if err != nil {
		return nil, fmt.Errorf("pbkdf2: %w", err)
	}
	defer zeroBytes(kek)

	encryptedKey, err := decodeJWEProtectedHeader(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode encrypted key: %w", err)
	}
	cek, err := aesKeyUnwrap(kek, encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("unwrap cek: %w", err)
	}
	defer zeroBytes(cek)

	iv, err := decodeJWEProtectedHeader(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode iv: %w", err)
	}
	ciphertext, err := decodeJWEProtectedHeader(parts[3])
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	tag, err := decodeJWEProtectedHeader(parts[4])
	if err != nil {
		return nil, fmt.Errorf("decode tag: %w", err)
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(iv) != gcm.NonceSize() || len(tag) != gcm.Overhead() {
		return nil, fmt.Errorf("invalid gcm iv/tag size")
	}
	sealed := make([]byte, 0, len(ciphertext)+len(tag))
	sealed = append(sealed, ciphertext...)
	sealed = append(sealed, tag...)
	return gcm.Open(nil, iv, sealed, []byte(parts[0]))
}

var rfc3394DefaultIV = [8]byte{0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6}

func aesKeyUnwrap(kek, wrapped []byte) ([]byte, error) {
	if len(wrapped) != 40 {
		return nil, fmt.Errorf("unexpected wrapped key length %d", len(wrapped))
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	const n = 4
	var a [8]byte
	copy(a[:], wrapped[:8])
	var r [n][8]byte
	for i := 0; i < n; i++ {
		copy(r[i][:], wrapped[(i+1)*8:(i+2)*8])
	}
	var buf [16]byte
	for j := 5; j >= 0; j-- {
		for i := n - 1; i >= 0; i-- {
			t := uint64(n*j + (i + 1))
			binary.BigEndian.PutUint64(buf[:8], binary.BigEndian.Uint64(a[:])^t)
			copy(buf[8:], r[i][:])
			block.Decrypt(buf[:], buf[:])
			copy(a[:], buf[:8])
			copy(r[i][:], buf[8:])
		}
	}
	if subtle.ConstantTimeCompare(a[:], rfc3394DefaultIV[:]) != 1 {
		return nil, fmt.Errorf("key unwrap integrity check failed")
	}
	out := make([]byte, 32)
	for i := 0; i < n; i++ {
		copy(out[i*8:(i+1)*8], r[i][:])
	}
	return out, nil
}

func parseProvisionerECJWK(data []byte) (*provisionerJWK, error) {
	var jwk provisionerJWK
	if err := json.Unmarshal(data, &jwk); err != nil {
		return nil, err
	}
	if jwk.Kty != "EC" || jwk.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported JWK kty=%q crv=%q", jwk.Kty, jwk.Crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil || len(xBytes) == 0 || len(xBytes) > 32 {
		return nil, fmt.Errorf("invalid JWK x coordinate")
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil || len(yBytes) == 0 || len(yBytes) > 32 {
		return nil, fmt.Errorf("invalid JWK y coordinate")
	}
	dBytes, err := base64.RawURLEncoding.DecodeString(jwk.D)
	if err != nil || len(dBytes) == 0 || len(dBytes) > 32 {
		return nil, fmt.Errorf("invalid JWK d parameter")
	}
	curve := elliptic.P256()
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	d := new(big.Int).SetBytes(dBytes)
	if d.Sign() <= 0 || d.Cmp(curve.Params().N) >= 0 {
		return nil, fmt.Errorf("JWK d out of range")
	}
	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("JWK point not on P-256 curve")
	}
	jwk.Key = &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
			X:     x,
			Y:     y,
		},
		D: d,
	}
	return &jwk, nil
}
