package viewerchat

import (
	"context"
	"encoding/json"
)

// TripCard is a compact trip summary for tools/prompt.
type TripCard struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description,omitempty"`
	Start         string   `json:"start,omitempty"`
	Days          int      `json:"days"`
	SchemaVersion int      `json:"schema_version"`
	DayTitles     []string `json:"day_titles,omitempty"`
	VersionID     string   `json:"version_id,omitempty"`
}

// MutateResult is returned after a successful itinerary mutation.
type MutateResult struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id,omitempty"`
	BundleOK  bool   `json:"bundle_ok"`
}

// VersionEntry is one prior YAML revision.
type VersionEntry struct {
	VersionID    string `json:"version_id"`
	LastModified string `json:"last_modified,omitempty"` // RFC3339
	IsLatest     bool   `json:"is_latest,omitempty"`
}

// TripOps is the in-process trip surface chat tools call (scoped by trip ID).
type TripOps interface {
	Summary(ctx context.Context, tripID string) (TripCard, error)
	SchemaJSON(ctx context.Context) (json.RawMessage, error)
	GetYAML(ctx context.Context, tripID string, scope string, day int) ([]byte, error)
	GetYAMLVersion(ctx context.Context, tripID, versionID string) ([]byte, error)
	Patch(ctx context.Context, tripID string, patchJSON []byte) (MutateResult, error)
	ReplaceDayRoutes(ctx context.Context, tripID string, bodyJSON []byte) (MutateResult, error)
	ListVersions(ctx context.Context, tripID string) ([]VersionEntry, error)
	RestoreVersion(ctx context.Context, tripID, versionID string) (MutateResult, error)
}
