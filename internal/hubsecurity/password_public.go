package hubsecurity

import (
	"crypto/rand"
	"fmt"
)

// HashLocalPassword applies the same Argon2id policy used by first-OWNER
// bootstrap. Plaintext passwords are never persisted by this package.
func HashLocalPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", fmt.Errorf("password must be at least 12 characters")
	}
	return hashPassword(password, rand.Reader)
}
