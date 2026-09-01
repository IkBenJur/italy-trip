package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// GenerateRefreshToken returns a high-entropy opaque token. Unlike the JWT
// access token it carries no claims - it's a bearer secret looked up against
// its hash in the database, so a single one can be revoked without touching
// any other session.
func GenerateRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashRefreshToken hashes a refresh token for storage. The token is already
// 256 bits of random entropy rather than a low-entropy secret someone could
// guess, so a fast hash is enough here - unlike passwords, it doesn't need
// bcrypt's deliberate slowness.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
