package itinerary

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// CurrentSchemaVersion is the only schema_version accepted on write.
const CurrentSchemaVersion = 2

// ParseYAML unmarshals itinerary YAML.
func ParseYAML(b []byte) (Trip, error) {
	var t Trip
	if err := yaml.Unmarshal(b, &t); err != nil {
		return Trip{}, fmt.Errorf("parse yaml: %w", err)
	}
	return t, nil
}

// MarshalYAML encodes a trip as YAML with a trailing newline.
func MarshalYAML(t Trip) ([]byte, error) {
	b, err := yaml.Marshal(&t)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// EnsureSchemaVersion sets schema_version to current if missing (0), or
// rejects unknown versions.
func EnsureSchemaVersion(t *Trip) error {
	if t.SchemaVersion == 0 {
		t.SchemaVersion = CurrentSchemaVersion
		return nil
	}
	if t.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d (want %d)", t.SchemaVersion, CurrentSchemaVersion)
	}
	return nil
}

// ValidateBasic checks required fields for storage (place refs, not coords).
func ValidateBasic(t Trip) error {
	if t.Trip == "" {
		return fmt.Errorf("trip title is required")
	}
	if len(t.Days) == 0 {
		return fmt.Errorf("at least one day is required")
	}
	if len(t.Places) == 0 {
		return fmt.Errorf("places catalog is required")
	}
	for id, p := range t.Places {
		if id == "" {
			return fmt.Errorf("place id must be non-empty")
		}
		if p.Title == "" {
			return fmt.Errorf("place %q: title is required", id)
		}
	}
	for _, d := range t.Days {
		if d.Day < 1 {
			return fmt.Errorf("invalid day number %d", d.Day)
		}
		if d.Title == "" {
			return fmt.Errorf("day %d title is required", d.Day)
		}
		for _, s := range append(append([]Stop{}, d.Route...), d.Stops...) {
			if s.Place == "" {
				return fmt.Errorf("day %d: stop missing place id", d.Day)
			}
			if _, ok := t.Places[s.Place]; !ok {
				return fmt.Errorf("day %d: unknown place %q", d.Day, s.Place)
			}
		}
	}
	return nil
}
