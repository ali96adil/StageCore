package id

import (
	"fmt"

	"github.com/google/uuid"
)

// New returns a canonical lowercase UUIDv7 string.
func New() (string, error) {
	v, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return v.String(), nil
}

// ValidateCanonical rejects non-canonical UUID spellings even when a parser could accept them.
func ValidateCanonical(value string) error {
	v, err := uuid.Parse(value)
	if err != nil {
		return fmt.Errorf("parse UUID: %w", err)
	}
	if v.String() != value {
		return fmt.Errorf("UUID is not canonical lowercase text")
	}
	return nil
}
