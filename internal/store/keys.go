package store

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

const maxIdempotencyKeyLen = 128

// ValidateIdempotencyKey rejects keys that could escape the idempotency/ S3 prefix
// via path.Join cleaning (e.g. "../../trips/…").
func ValidateIdempotencyKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("empty idempotency key")
	}
	if len(key) > maxIdempotencyKeyLen {
		return fmt.Errorf("idempotency key too long (max %d)", maxIdempotencyKeyLen)
	}
	if strings.Contains(key, "..") || strings.ContainsAny(key, `/\`) {
		return fmt.Errorf("invalid idempotency key")
	}
	for _, r := range key {
		switch {
		case r == '.' || r == '_' || r == '-' || r == ':':
		case unicode.IsLetter(r) || unicode.IsDigit(r):
		default:
			return fmt.Errorf("invalid idempotency key")
		}
	}
	return nil
}

func yamlKey(id string) string {
	return path.Join("trips", id, "itinerary.yaml")
}

func metaKey(id string) string {
	return path.Join("trips", id, "meta.json")
}

func bundlePrefix(id string) string {
	return path.Join("trips", id, "bundle") + "/"
}

func idemKey(key string) (string, error) {
	if err := ValidateIdempotencyKey(key); err != nil {
		return "", err
	}
	// Concatenate (not path.Join) so a validated key cannot be re-cleaned into
	// another prefix. Keys must not contain '/'.
	return "idempotency/" + key, nil
}

func notesKey(id string) string {
	return path.Join("trips", id, "notes.json")
}
