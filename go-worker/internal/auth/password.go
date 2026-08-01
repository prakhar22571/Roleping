package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
)

// DefaultIterations is the PBKDF2 work factor used for new passwords.
//
// This is deliberately far below the usual OWASP recommendation because
// Cloudflare's free plan caps a Worker invocation at ~10ms of CPU, and
// PBKDF2 running in WASM burns that budget quickly - too high a value makes
// login fail outright rather than merely feel slow. The per-user
// `password_iterations` column records what each hash was built with, so
// this can be raised later without invalidating existing passwords.
//
// What carries the security here is password entropy, not the work factor:
// passwords are generated (see GeneratePassword) with ~103 bits of entropy,
// which is not brute-forceable regardless of iteration count. If you start
// letting users choose their own passwords, revisit this.
const DefaultIterations = 20000

const (
	saltLen = 16
	keyLen  = 32
)

var ErrNoPassword = errors.New("account has no password set")

// pbkdf2SHA256 implements PBKDF2-HMAC-SHA256 (RFC 8018) for a single output
// block, which is all a 32-byte key needs. Implemented here rather than
// pulling in golang.org/x/crypto to keep the WASM bundle small.
func pbkdf2SHA256(password, salt []byte, iterations int) []byte {
	mac := hmac.New(sha256.New, password)

	// U1 = PRF(password, salt || INT_BE(1))
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1})
	u := mac.Sum(nil)

	out := make([]byte, len(u))
	copy(out, u)

	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(u[:0])
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

type PasswordHash struct {
	Hash       string
	Salt       string
	Iterations int
}

func HashPassword(password string) (PasswordHash, error) {
	salt, err := randomBytes(saltLen)
	if err != nil {
		return PasswordHash{}, err
	}
	key := pbkdf2SHA256([]byte(password), salt, DefaultIterations)
	return PasswordHash{
		Hash:       base64.StdEncoding.EncodeToString(key),
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Iterations: DefaultIterations,
	}, nil
}

// VerifyPassword reports whether password matches the stored hash. It is
// constant-time with respect to the hash contents.
func VerifyPassword(stored PasswordHash, password string) bool {
	if stored.Hash == "" || stored.Salt == "" || stored.Iterations <= 0 {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(stored.Salt)
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(stored.Hash)
	if err != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, stored.Iterations)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// passwordAlphabet omits look-alike characters (0/O, 1/l/I) so a generated
// password can be read off a screen and retyped without ambiguity.
const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GeneratePassword returns a random password of n characters
// (~5.8 bits each; the default 18 gives ~103 bits of entropy).
func GeneratePassword(n int) (string, error) {
	if n <= 0 {
		n = 18
	}
	out := make([]byte, n)
	// Rejection-sample to keep the distribution uniform: 256 is not a
	// multiple of the alphabet length, so accepting every byte modulo it
	// would bias the low characters.
	limit := byte(256 - (256 % len(passwordAlphabet)))
	for i := 0; i < n; {
		b, err := randomBytes(1)
		if err != nil {
			return "", err
		}
		if b[0] >= limit {
			continue
		}
		out[i] = passwordAlphabet[int(b[0])%len(passwordAlphabet)]
		i++
	}
	return string(out), nil
}
