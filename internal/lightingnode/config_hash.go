package lightingnode

import (
	"crypto/sha256"
	"encoding/hex"
)

func ConfigurationHash(config Configuration) (string, error) {
	canonical, err := CanonicalConfiguration(config)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
