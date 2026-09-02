package executionenv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// DecodeCanonical decodes a stored manifest only when its bytes are exactly the
// canonical representation produced by CanonicalBytes. This keeps persisted
// manifest identity content-addressable and rejects whitespace/order drift,
// unknown fields, and other storage tampering instead of silently normalizing
// corrupted durable state on read.
func DecodeCanonical(payload []byte) (Manifest, error) {
	if len(payload) == 0 {
		return Manifest{}, fmt.Errorf("execution environment manifest is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode execution environment manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Manifest{}, err
	}
	normalized, err := Normalize(manifest)
	if err != nil {
		return Manifest{}, fmt.Errorf("validate execution environment manifest: %w", err)
	}
	canonical, err := CanonicalBytes(normalized)
	if err != nil {
		return Manifest{}, err
	}
	if !bytes.Equal(payload, canonical) {
		return Manifest{}, fmt.Errorf("execution environment manifest bytes are not canonical")
	}
	return normalized, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode execution environment trailing data: %w", err)
	}
	return fmt.Errorf("execution environment manifest contains trailing JSON data")
}
