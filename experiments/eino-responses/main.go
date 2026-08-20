// Eino Responses API multi-turn smoke — compare to ADK-Go #1197.
// No MCP; proves whether stock Eino survives turn 2 without an input_text rewrite.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/schema"
)

func main() {
	cache := flag.Bool("cache", true, "EnableAutoCache (previous_response_id path)")
	logPath := flag.String("log", "", "JSONL path for captured /v1/responses bodies (default stdout)")
	flag.Parse()

	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("set OPENAI_API_KEY")
	}
	modelName := envOr("OPENAI_MODEL", "gpt-5-mini")

	out := os.Stdout
	if *logPath != "" {
		f, err := os.Create(*logPath)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	cap := &captureTransport{Base: http.DefaultTransport, Enc: enc}
	httpClient := &http.Client{Transport: cap, Timeout: 120 * time.Second}

	ctx := context.Background()
	am, err := agenticopenai.NewResponsesModel(ctx, &agenticopenai.ResponsesConfig{
		APIKey:          apiKey,
		Model:           modelName,
		HTTPClient:      httpClient,
		EnableAutoCache: *cache,
	})
	if err != nil {
		log.Fatalf("NewResponsesModel: %v", err)
	}

	_ = enc.Encode(map[string]any{
		"type":             "run_start",
		"ts":               time.Now().UTC().Format(time.RFC3339Nano),
		"framework":        "eino",
		"model":            modelName,
		"enable_auto_cache": *cache,
		"purpose":          "multi-turn Responses smoke vs ADK #1197",
	})

	msgs := []*schema.AgenticMessage{
		schema.UserAgenticMessage("Reply with exactly: turn-one. No tools."),
	}

	start := time.Now()
	asst1, err := am.Generate(ctx, msgs)
	if err != nil {
		_ = enc.Encode(map[string]any{"type": "error", "turn": 1, "error": err.Error()})
		log.Fatalf("turn 1: %v", err)
	}
	msgs = append(msgs, asst1)
	text1 := assistantText(asst1)
	id1 := responseID(asst1)
	fmt.Fprintf(os.Stderr, "turn1 ok text=%q response_id=%q\n", truncate(text1, 80), id1)
	_ = enc.Encode(map[string]any{
		"type":        "turn_ok",
		"turn":        1,
		"text":        text1,
		"response_id": id1,
		"elapsed_ms":  time.Since(start).Milliseconds(),
	})

	msgs = append(msgs, schema.UserAgenticMessage("Reply with exactly: turn-two. No tools."))
	asst2, err := am.Generate(ctx, msgs)
	if err != nil {
		_ = enc.Encode(map[string]any{"type": "error", "turn": 2, "error": err.Error()})
		// Explicit ADK-parity failure signal
		if strings.Contains(err.Error(), "input_text") {
			_ = enc.Encode(map[string]any{
				"type":   "verdict",
				"pass":   false,
				"reason": "ADK-like input_text rejection on turn 2",
			})
		}
		log.Fatalf("turn 2: %v", err)
	}
	text2 := assistantText(asst2)
	id2 := responseID(asst2)
	fmt.Fprintf(os.Stderr, "turn2 ok text=%q response_id=%q\n", truncate(text2, 80), id2)
	_ = enc.Encode(map[string]any{
		"type":        "turn_ok",
		"turn":        2,
		"text":        text2,
		"response_id": id2,
		"elapsed_ms":  time.Since(start).Milliseconds(),
	})

	verdict := analyzeCaptures(cap.Snapshots())
	verdict["pass"] = true
	verdict["type"] = "verdict"
	verdict["turn2_text"] = text2
	_ = enc.Encode(verdict)
	fmt.Fprintf(os.Stderr, "verdict: %v\n", verdict)
}

type captureTransport struct {
	Base http.RoundTripper
	Enc  *json.Encoder
	mu   sync.Mutex
	snaps []map[string]any
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	var body []byte
	if req.Body != nil && isResponses(req) {
		var err error
		body, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		snap := summarizeRequest(body)
		t.mu.Lock()
		t.snaps = append(t.snaps, snap)
		t.mu.Unlock()
		_ = t.Enc.Encode(map[string]any{
			"type":    "responses_request",
			"n":       len(t.snaps),
			"summary": snap,
			// Full body can be large; keep for debugging when small.
			"body": json.RawMessage(body),
		})
	}
	return resp, nil
}

func (t *captureTransport) Snapshots() []map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]map[string]any, len(t.snaps))
	copy(out, t.snaps)
	return out
}

func isResponses(req *http.Request) bool {
	if req.URL == nil {
		return false
	}
	return strings.Contains(req.URL.Path, "/responses")
}

func summarizeRequest(body []byte) map[string]any {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return map[string]any{"parse_error": err.Error()}
	}
	sum := map[string]any{}
	if v, ok := root["previous_response_id"]; ok {
		sum["previous_response_id"] = v
	}
	if v, ok := root["store"]; ok {
		sum["store"] = v
	}
	input := root["input"]
	sum["input_kind"] = fmt.Sprintf("%T", input)
	var assistantInputText, assistantOutputText, userInputText int
	var roles []string
	walkInput(input, &roles, &assistantInputText, &assistantOutputText, &userInputText)
	sum["roles"] = roles
	sum["assistant_input_text_blocks"] = assistantInputText
	sum["assistant_output_text_blocks"] = assistantOutputText
	sum["user_input_text_blocks"] = userInputText
	return sum
}

func walkInput(input any, roles *[]string, asstIn, asstOut, userIn *int) {
	arr, ok := input.([]any)
	if !ok {
		return
	}
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		typ, _ := m["type"].(string)
		*roles = append(*roles, role+"/"+typ)
		content := m["content"]
		switch c := content.(type) {
		case string:
			// plain-string easy message — legal for assistant
			if strings.EqualFold(role, "assistant") {
				*asstOut++ // count as non-input_text assistant content
			} else if strings.EqualFold(role, "user") {
				*userIn++
			}
		case []any:
			for _, block := range c {
				bm, ok := block.(map[string]any)
				if !ok {
					continue
				}
				bt, _ := bm["type"].(string)
				switch {
				case strings.EqualFold(role, "assistant") && bt == "input_text":
					*asstIn++
				case strings.EqualFold(role, "assistant") && bt == "output_text":
					*asstOut++
				case strings.EqualFold(role, "user") && bt == "input_text":
					*userIn++
				}
			}
		}
	}
}

func analyzeCaptures(snaps []map[string]any) map[string]any {
	out := map[string]any{
		"request_count": len(snaps),
	}
	var usedPrevID bool
	var badAssistantInputText bool
	var goodAssistantReplay bool
	for i, s := range snaps {
		if v, ok := s["previous_response_id"]; ok && v != nil && fmt.Sprint(v) != "" {
			usedPrevID = true
			out[fmt.Sprintf("req_%d_previous_response_id", i+1)] = v
		}
		if n, _ := s["assistant_input_text_blocks"].(int); n > 0 {
			badAssistantInputText = true
		}
		if n, _ := s["assistant_output_text_blocks"].(int); n > 0 {
			goodAssistantReplay = true
		}
	}
	out["used_previous_response_id"] = usedPrevID
	out["had_assistant_input_text"] = badAssistantInputText
	out["had_assistant_output_or_string"] = goodAssistantReplay
	switch {
	case usedPrevID:
		out["strategy"] = "previous_response_id (server-side chain)"
	case goodAssistantReplay && !badAssistantInputText:
		out["strategy"] = "client history with output_text/string"
	case badAssistantInputText:
		out["strategy"] = "client history with assistant input_text (ADK bug class)"
	default:
		out["strategy"] = "unknown/minimal input"
	}
	return out
}

func assistantText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range msg.ContentBlocks {
		if block.AssistantGenText != nil {
			b.WriteString(block.AssistantGenText.Text)
		}
	}
	return b.String()
}

func responseID(msg *schema.AgenticMessage) string {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.OpenAIExtension == nil {
		return ""
	}
	return msg.ResponseMeta.OpenAIExtension.ID
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
