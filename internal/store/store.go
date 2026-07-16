package store

import (
	"context"
	"time"
)

// Meta is trips/{id}/meta.json (never stores plaintext capability tokens).
type Meta struct {
	SchemaVersion int       `json:"schema_version"`
	TokenHash     string    `json:"token_hash"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// VersionInfo is one S3 object version of the YAML.
type VersionInfo struct {
	VersionID    string    `json:"version_id"`
	LastModified time.Time `json:"last_modified"`
	IsLatest     bool      `json:"is_latest"`
}

// YAMLObject is itinerary YAML plus optional version id.
type YAMLObject struct {
	Body      []byte
	VersionID string
}

// Store is the itineraries (+ comments) bucket abstraction.
type Store interface {
	ListTripIDs(ctx context.Context) ([]string, error)
	GetYAML(ctx context.Context, id string) (YAMLObject, error)
	GetYAMLVersion(ctx context.Context, id, versionID string) (YAMLObject, error)
	PutYAML(ctx context.Context, id string, body []byte) (versionID string, err error)
	GetMeta(ctx context.Context, id string) (Meta, error)
	PutMeta(ctx context.Context, id string, m Meta) error
	ListVersions(ctx context.Context, id string) ([]VersionInfo, error)
	GetIdempotency(ctx context.Context, key string) ([]byte, bool, error)
	PutIdempotency(ctx context.Context, key string, body []byte) error
	UploadBundle(ctx context.Context, id string, root string) error
	GetBundleObject(ctx context.Context, id, rel string) (body []byte, contentType string, err error)
	GetNotes(ctx context.Context, id string) ([]byte, error)
	PutNotes(ctx context.Context, id string, body []byte) error
	Exists(ctx context.Context, id string) (bool, error)
}

// NotesDoc is the shared notes JSON shape (also used by the viewer).
type NotesDoc struct {
	Days      map[string]string `json:"days"`
	UpdatedAt time.Time         `json:"updated_at"`
}
