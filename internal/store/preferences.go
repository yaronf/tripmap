package store

import (
	"fmt"
	"path"
	"strings"
	"time"
)

const (
	MaxPreferenceItems = 32
	MaxPreferenceText  = 500
)

// PreferenceItem is one standing user preference for viewer chat.
type PreferenceItem struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PreferencesDoc is users/{sub}/preferences.json in the comments bucket.
type PreferencesDoc struct {
	UpdatedAt time.Time        `json:"updated_at"`
	Items     []PreferenceItem `json:"items"`
}

func preferencesKey(sub string) (string, error) {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return "", fmt.Errorf("empty user sub")
	}
	if len(sub) > 128 {
		return "", fmt.Errorf("user sub too long")
	}
	// Reject path traversal / separators only; Hellō subs are opaque strings.
	if strings.Contains(sub, "..") || strings.ContainsAny(sub, `/\`) {
		return "", fmt.Errorf("invalid user sub")
	}
	for _, r := range sub {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("invalid user sub")
		}
	}
	return path.Join("users", sub, "preferences.json"), nil
}

// EmptyPreferences returns a doc with an empty items slice.
func EmptyPreferences() PreferencesDoc {
	return PreferencesDoc{Items: []PreferenceItem{}}
}
