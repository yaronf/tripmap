package viewerchat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/yaronf/tripmap/internal/itinerary"
)

type memOps struct {
	card      TripCard
	schema    json.RawMessage
	yaml      []byte
	patches   int
	lastPatch json.RawMessage
	prefs     []Preference
	learnings []Learning
}

func (m *memOps) Summary(context.Context, string) (TripCard, error) { return m.card, nil }
func (m *memOps) SchemaJSON(context.Context) (json.RawMessage, error) {
	if m.schema == nil {
		return json.RawMessage(`{"ok":true}`), nil
	}
	return m.schema, nil
}
func (m *memOps) GetYAML(context.Context, string) ([]byte, error) { return m.yaml, nil }
func (m *memOps) GetYAMLVersion(_ context.Context, _ string, versionID string) ([]byte, error) {
	return []byte("trip: historical\nversion: " + versionID + "\n"), nil
}
func (m *memOps) GetDay(_ context.Context, _ string, day int) (DayDetail, error) {
	return DayDetailFromYAML(m.yaml, day)
}
func (m *memOps) Patch(_ context.Context, _ string, patchJSON []byte) (PatchResult, error) {
	m.patches++
	m.lastPatch = append(json.RawMessage(nil), patchJSON...)
	if len(m.yaml) > 0 {
		trip, err := itinerary.ParseYAML(m.yaml)
		if err != nil {
			return PatchResult{}, err
		}
		var p itinerary.Patch
		if err := json.Unmarshal(patchJSON, &p); err != nil {
			return PatchResult{}, err
		}
		if err := itinerary.ApplyPatch(&trip, p); err != nil {
			return PatchResult{}, err
		}
		out, err := itinerary.MarshalYAML(trip)
		if err != nil {
			return PatchResult{}, err
		}
		m.yaml = out
	}
	return PatchResult{ID: "t1", VersionID: "v1", BundleOK: true}, nil
}

func (m *memOps) CommitYAML(_ context.Context, _ string, yamlBody []byte) (PatchResult, error) {
	m.patches++
	trip, err := itinerary.ParseYAML(yamlBody)
	if err != nil {
		return PatchResult{}, err
	}
	out, err := itinerary.MarshalYAML(trip)
	if err != nil {
		return PatchResult{}, err
	}
	m.yaml = out
	return PatchResult{ID: "t1", VersionID: "v1", BundleOK: true}, nil
}

func (m *memOps) ListVersions(context.Context, string) ([]VersionEntry, error) {
	return []VersionEntry{
		{VersionID: "v1", LastModified: "2026-08-10T12:00:00Z", IsLatest: true},
	}, nil
}

func (m *memOps) RestoreVersion(_ context.Context, _ string, versionID string) (PatchResult, error) {
	return PatchResult{ID: "t1", VersionID: "v2-from-" + versionID, BundleOK: true}, nil
}

func (m *memOps) ListPreferences(context.Context, string) ([]Preference, error) {
	return append([]Preference(nil), m.prefs...), nil
}

func (m *memOps) SavePreference(_ context.Context, _, id, text string, tags []string) (Preference, error) {
	if id == "" {
		id = "pref_test"
	}
	p := Preference{ID: id, Text: text, Tags: tags}
	for i := range m.prefs {
		if m.prefs[i].ID == id {
			m.prefs[i] = p
			return p, nil
		}
	}
	m.prefs = append(m.prefs, p)
	return p, nil
}

func (m *memOps) ForgetPreference(_ context.Context, _, id string) error {
	out := make([]Preference, 0, len(m.prefs))
	for _, p := range m.prefs {
		if p.ID == id {
			continue
		}
		out = append(out, p)
	}
	m.prefs = out
	return nil
}

func (m *memOps) ListLearnings(context.Context, string) ([]Learning, error) {
	return append([]Learning(nil), m.learnings...), nil
}

func (m *memOps) SaveLearning(_ context.Context, _, id, text string, tags []string) (Learning, error) {
	if id == "" {
		id = "learn_test"
	}
	p := Learning{ID: id, Text: text, Tags: tags}
	for i := range m.learnings {
		if m.learnings[i].ID == id {
			m.learnings[i] = p
			return p, nil
		}
	}
	m.learnings = append(m.learnings, p)
	return p, nil
}

func (m *memOps) ForgetLearning(_ context.Context, _, id string) error {
	out := make([]Learning, 0, len(m.learnings))
	for _, p := range m.learnings {
		if p.ID == id {
			continue
		}
		out = append(out, p)
	}
	m.learnings = out
	return nil
}

func TestNewAgentDefaultModelIsGPT4o(t *testing.T) {
	a := NewAgent(Config{APIKey: "sk-test", Ops: &memOps{}})
	if a.model != string(openai.ChatModelGPT4o) {
		t.Fatalf("model=%q", a.model)
	}
}

func TestAgentToolLoopPatchesOnce(t *testing.T) {
	ops := &memOps{card: TripCard{ID: "t1", Title: "Test", Days: 2}}

	// Build responses with proper JSON so AsFunctionCall / OutputText work.
	var callResp responses.Response
	if err := json.Unmarshal([]byte(`{
		"id":"resp_1",
		"output":[{
			"type":"function_call",
			"id":"fc_1",
			"call_id":"call1",
			"name":"patchTrip",
			"arguments":"{\"update_day\":{\"day\":1,\"title\":\"New\"}}"
		}]
	}`), &callResp); err != nil {
		t.Fatal(err)
	}
	var textResp responses.Response
	if err := json.Unmarshal([]byte(`{
		"id":"resp_2",
		"output":[{
			"type":"message",
			"role":"assistant",
			"content":[{"type":"output_text","text":"Updated day 1."}]
		}]
	}`), &textResp); err != nil {
		t.Fatal(err)
	}
	step := 0
	a := &Agent{
		model: "test",
		ops:   ops,
		respond: func(_ context.Context, _ responses.ResponseNewParams) (*responses.Response, error) {
			step++
			if step == 1 {
				return &callResp, nil
			}
			return &textResp, nil
		},
	}

	var texts []string
	res, err := a.run(context.Background(), TurnInput{
		TripID:   "t1",
		Messages: []ClientMessage{{Role: "user", Content: "Rename day 1"}},
		Day:      1,
	}, func(ev Event) error {
		if ev.Type == "text" {
			texts = append(texts, ev.Text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ops.patches != 1 {
		t.Fatalf("patches=%d want 1", ops.patches)
	}
	if !res.TripUpdated {
		t.Fatal("expected trip updated")
	}
	if len(texts) != 1 || texts[0] != "Updated day 1." {
		t.Fatalf("texts=%v", texts)
	}
}

func TestChatToolsIncludeWebSearch(t *testing.T) {
	tools := chatTools()
	var hasWeb, hasPatch, hasPreview bool
	for _, tool := range tools {
		if tool.OfWebSearchPreview != nil {
			hasPreview = true
		}
		if tool.OfWebSearch != nil {
			hasWeb = true
		}
		if tool.OfFunction != nil && tool.OfFunction.Name == "patchTrip" {
			hasPatch = true
		}
	}
	if !hasWeb || !hasPatch || hasPreview {
		t.Fatalf("tools web=%v patch=%v preview=%v", hasWeb, hasPatch, hasPreview)
	}
}

func TestHandlerRejectsEmptyBody(t *testing.T) {
	h := &Handler{Agent: &Agent{ops: &memOps{}, respond: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
		t.Fatal("should not call openai")
		return nil, nil
	}}}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req, "t1", "sub1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}
