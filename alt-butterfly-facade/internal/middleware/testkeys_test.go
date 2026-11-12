package middleware

import (
	"crypto/ed25519"
	"crypto/rand"
)

var testPrivateKey = func() ed25519.PrivateKey {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return priv
}()

var testPublicKey = testPrivateKey.Public().(ed25519.PublicKey)
