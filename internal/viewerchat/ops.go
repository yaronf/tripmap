package viewerchat

import (
	"context"
	"encoding/json"
)

// TripCard is a compact trip summary for the system prompt.
type TripCard struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description,omitempty"`
	Start         string   `json:"start,omitempty"`
	Days          int      `json:"days"`
	SchemaVersion int      `json:"schema_version"`
	DayTitles     []string `json:"day_titles,omitempty"`
}

// PatchResult is returned after a successful itinerary mutation.
type PatchResult struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id,omitempty"`
	BundleOK  bool   `json:"bundle_ok"`
}

// TripOps is the in-process trip surface chat tools call (scoped by trip ID).
type TripOps interface {
	Summary(ctx context.Context, tripID string) (TripCard, error)
	SchemaJSON(ctx context.Context) (json.RawMessage, error)
	GetYAML(ctx context.Context, tripID string) ([]byte, error)
	Patch(ctx context.Context, tripID string, patchJSON []byte) (PatchResult, error)
}
