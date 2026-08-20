package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// fixResponsesAssistantContent wraps an HTTP RoundTripper so OpenAI Responses
// multi-turn calls succeed with ADK-Go openaimodel (v2.1.0–v2.2.0 at least).
//
// Upstream: https://github.com/google/adk-go/issues/1197
// Pending fixes: https://github.com/google/adk-go/pull/1205 ,
// https://github.com/google/adk-go/pull/1291 (not released as of 2026-08-20).
//
// ADK convertContents/newMessage emits typed content type "input_text" for every
// role. Responses API rejects that under role=assistant (needs "output_text" or
// a plain-string easy message). One-shot prompts never hit this; turn 2+ does.
//
// This rewrite only flips the type tag on the wire. It does NOT preserve
// reasoning items (encrypted_content / phase) for native Responses chaining.
// Prefer an official ADK release or PR #1205-style OfOutputMessage once merged.
type fixResponsesAssistantContent struct {
	Base http.RoundTripper
}

func (t *fixResponsesAssistantContent) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if req.Method == http.MethodPost && req.Body != nil && isResponsesAPI(req) {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		fixed, changed := rewriteAssistantInputText(body)
		if changed {
			body = fixed
		}
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return base.RoundTrip(req)
}

func isResponsesAPI(req *http.Request) bool {
	if req.URL == nil {
		return false
	}
	p := req.URL.Path
	return strings.HasSuffix(p, "/responses") || strings.Contains(p, "/v1/responses")
}

// rewriteAssistantInputText walks a Responses API request JSON and renames
// content blocks type input_text → output_text under role=assistant messages.
func rewriteAssistantInputText(raw []byte) ([]byte, bool) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw, false
	}
	input, ok := root["input"]
	if !ok {
		return raw, false
	}
	changed := false
	switch v := input.(type) {
	case []any:
		for _, item := range v {
			if rewriteInputItem(item) {
				changed = true
			}
		}
	default:
		return raw, false
	}
	if !changed {
		return raw, false
	}
	out, err := json.Marshal(root)
	if err != nil {
		return raw, false
	}
	return out, true
}

func rewriteInputItem(item any) bool {
	m, ok := item.(map[string]any)
	if !ok {
		return false
	}
	role, _ := m["role"].(string)
	if !strings.EqualFold(role, "assistant") {
		return false
	}
	content, ok := m["content"]
	if !ok {
		return false
	}
	return rewriteContentList(content)
}

func rewriteContentList(content any) bool {
	arr, ok := content.([]any)
	if !ok {
		return false
	}
	changed := false
	for _, c := range arr {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _ := cm["type"].(string)
		if t == "input_text" {
			cm["type"] = "output_text"
			changed = true
		}
	}
	return changed
}
