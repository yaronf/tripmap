package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Mem is an in-memory Store for tests.
type Mem struct {
	mu   sync.Mutex
	yaml map[string][]yamlVer // id -> versions (last is latest)
	meta map[string]Meta
	idem map[string][]byte
	bund map[string]map[string][]byte // id -> relpath -> bytes
}

type yamlVer struct {
	id   string
	body []byte
	at   time.Time
}

// NewMem returns an empty memory store.
func NewMem() *Mem {
	return &Mem{
		yaml: map[string][]yamlVer{},
		meta: map[string]Meta{},
		idem: map[string][]byte{},
		bund: map[string]map[string][]byte{},
	}
}

func (m *Mem) ListTripIDs(ctx context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.yaml))
	for id := range m.yaml {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (m *Mem) Exists(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.yaml[id]
	return ok, nil
}

func (m *Mem) GetYAML(ctx context.Context, id string) (YAMLObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	vs := m.yaml[id]
	if len(vs) == 0 {
		return YAMLObject{}, fmt.Errorf("trip %q not found", id)
	}
	v := vs[len(vs)-1]
	return YAMLObject{Body: append([]byte(nil), v.body...), VersionID: v.id}, nil
}

func (m *Mem) GetYAMLVersion(ctx context.Context, id, versionID string) (YAMLObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.yaml[id] {
		if v.id == versionID {
			return YAMLObject{Body: append([]byte(nil), v.body...), VersionID: v.id}, nil
		}
	}
	return YAMLObject{}, fmt.Errorf("trip %q version %q not found", id, versionID)
}

func (m *Mem) PutYAML(ctx context.Context, id string, body []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.yaml[id]) + 1
	vid := fmt.Sprintf("v%d", n)
	m.yaml[id] = append(m.yaml[id], yamlVer{id: vid, body: append([]byte(nil), body...), at: time.Now().UTC()})
	return vid, nil
}

func (m *Mem) GetMeta(ctx context.Context, id string) (Meta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.meta[id]
	if !ok {
		return Meta{}, fmt.Errorf("meta for %q not found", id)
	}
	return meta, nil
}

func (m *Mem) PutMeta(ctx context.Context, id string, meta Meta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.meta[id] = meta
	return nil
}

func (m *Mem) ListVersions(ctx context.Context, id string) ([]VersionInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	vs := m.yaml[id]
	if len(vs) == 0 {
		return nil, fmt.Errorf("trip %q not found", id)
	}
	out := make([]VersionInfo, len(vs))
	for i, v := range vs {
		out[i] = VersionInfo{VersionID: v.id, LastModified: v.at, IsLatest: i == len(vs)-1}
	}
	// newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (m *Mem) GetIdempotency(ctx context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.idem[key]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), b...), true, nil
}

func (m *Mem) PutIdempotency(ctx context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idem[key] = append([]byte(nil), body...)
	return nil
}

func (m *Mem) UploadBundle(ctx context.Context, id string, root string) error {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bund[id] = files
	return nil
}

// BundleFiles returns uploaded bundle paths (tests).
func (m *Mem) BundleFiles(id string) map[string][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bund[id]
}

// EncodeJSON is a small helper used by handlers/tests.
func EncodeJSON(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	_ = enc.Encode(v)
	return buf.Bytes()
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

func idemKey(key string) string {
	return path.Join("idempotency", key)
}

func tripIDFromYAMLKey(key string) (string, bool) {
	// trips/{id}/itinerary.yaml
	parts := strings.Split(key, "/")
	if len(parts) != 3 || parts[0] != "trips" || parts[2] != "itinerary.yaml" {
		return "", false
	}
	return parts[1], true
}
