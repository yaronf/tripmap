package viewerchat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/responses"
)

type memOps struct {
	card      TripCard
	schema    json.RawMessage
	yaml      []byte
	patches   int
	lastPatch json.RawMessage
}

func (m *memOps) Summary(context.Context, string) (TripCard, error) { return m.card, nil }
func (m *memOps) SchemaJSON(context.Context) (json.RawMessage, error) {
	if m.schema == nil {
		return json.RawMessage(`{"ok":true}`), nil
	}
	return m.schema, nil
}
func (m *memOps) GetYAML(context.Context, string) ([]byte, error) { return m.yaml, nil }
func (m *memOps) GetDay(_ context.Context, _ string, day int) (DayDetail, error) {
	return DayDetailFromYAML(m.yaml, day)
}
func (m *memOps) Patch(_ context.Context, _ string, patchJSON []byte) (PatchResult, error) {
	m.patches++
	m.lastPatch = append(json.RawMessage(nil), patchJSON...)
	return PatchResult{ID: "t1", VersionID: "v1", BundleOK: true}, nil
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
			"name":"patch_trip",
			"arguments":"{\"patch\":{\"update_day\":{\"day\":1,\"title\":\"New\"}}}"
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
		if tool.OfFunction != nil && tool.OfFunction.Name == "patch_trip" {
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
	h.ServeHTTP(rec, req, "t1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}
