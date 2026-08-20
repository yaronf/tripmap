package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
)

// Multi-turn scenario file (suite-mt/scenarios/*.json).
type scenarioFile struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Hazard       string         `json:"hazard"`
	Setup        scenarioSetup  `json:"setup"`
	Turns        []string       `json:"turns"`
	Checks       []scenarioCheck `json:"checks"`
	PassNotes    string         `json:"pass_notes"`
}

type scenarioSetup struct {
	Trip           string `json:"trip"`
	RestoreVersion string `json:"restore_version"`
}

type scenarioCheck struct {
	Kind    string   `json:"kind"`
	Turn    int      `json:"turn"` // 0-based user turn index where relevant
	Tools   []string `json:"tools"`
	Pattern string   `json:"pattern"`
}

type turnTrace struct {
	Index      int
	User       string
	Texts      []string
	ToolNames  []string
	ToolArgs   []string // JSON of args per call
	ToolFails  []string
}

func loadScenario(path string) (*scenarioFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s scenarioFile
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.ID == "" || len(s.Turns) == 0 {
		return nil, fmt.Errorf("scenario %s: need id and turns", path)
	}
	return &s, nil
}

func runScenario(ctx context.Context, a agent.Agent, userID, scenarioPath, logPath string) error {
	sc, err := loadScenario(scenarioPath)
	if err != nil {
		return err
	}

	start := time.Now()
	out := os.Stdout
	if logPath != "" {
		if err := os.MkdirAll(dirOf(logPath), 0o755); err != nil {
			return err
		}
		f, err := os.Create(logPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	_ = enc.Encode(map[string]any{
		"type":      "scenario_start",
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"scenario":  sc.ID,
		"title":     sc.Title,
		"hazard":    sc.Hazard,
		"path":      scenarioPath,
		"turns":     len(sc.Turns),
		"model":     envOr("OPENAI_MODEL", defaultModel),
		"mcp_url":   envOr("TRIPMAP_MCP_URL", defaultMCPURL),
		"user_id":   userID,
		"setup":     sc.Setup,
		"pass_notes": sc.PassNotes,
	})

	if sc.Setup.Trip != "" && sc.Setup.RestoreVersion != "" {
		if err := restoreTripVersion(ctx, sc.Setup.Trip, sc.Setup.RestoreVersion); err != nil {
			_ = enc.Encode(map[string]any{"type": "setup_error", "error": err.Error()})
			return fmt.Errorf("setup restore: %w", err)
		}
		_ = enc.Encode(map[string]any{
			"type":           "setup_restore",
			"trip":           sc.Setup.Trip,
			"restore_version": sc.Setup.RestoreVersion,
		})
	}

	ss := session.InMemoryService()
	created, err := ss.Create(ctx, &session.CreateRequest{AppName: appName, UserID: userID})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	r, err := runner.New(runner.Config{AppName: appName, Agent: a, SessionService: ss})
	if err != nil {
		return fmt.Errorf("runner: %w", err)
	}

	var traces []turnTrace
	sessionID := created.Session.ID()
	_ = enc.Encode(map[string]any{"type": "session", "session_id": sessionID})

	for i, userText := range sc.Turns {
		turnStart := time.Now()
		tr := turnTrace{Index: i, User: userText}
		_ = enc.Encode(map[string]any{
			"type":       "turn_start",
			"turn":       i,
			"user":       userText,
			"elapsed_ms": time.Since(start).Milliseconds(),
		})

		msg := genai.NewContentFromText(userText, genai.RoleUser)
		var nToolCalls int
		for event, err := range r.Run(ctx, userID, sessionID, msg, agent.RunConfig{}) {
			rec := map[string]any{
				"type":       "event",
				"turn":       i,
				"ts":         time.Now().UTC().Format(time.RFC3339Nano),
				"elapsed_ms": time.Since(start).Milliseconds(),
			}
			if err != nil {
				rec["error"] = err.Error()
				_ = enc.Encode(rec)
				return fmt.Errorf("turn %d: %w", i, err)
			}
			rec["event_id"] = event.ID
			rec["author"] = event.Author
			if event.ErrorCode != "" || event.ErrorMessage != "" {
				rec["error_code"] = event.ErrorCode
				rec["error_message"] = event.ErrorMessage
				tr.ToolFails = append(tr.ToolFails, event.ErrorMessage)
			}
			if event.Content != nil {
				sum := summarizeContent(event.Content, &nToolCalls)
				rec["content"] = sum
				if t, ok := sum["text"].(string); ok && t != "" {
					tr.Texts = append(tr.Texts, t)
				}
				if calls, ok := sum["function_calls"].([]map[string]any); ok {
					for _, c := range calls {
						name, _ := c["name"].(string)
						tr.ToolNames = append(tr.ToolNames, name)
						if args, ok := c["args"]; ok {
							b, _ := json.Marshal(args)
							tr.ToolArgs = append(tr.ToolArgs, string(b))
						}
					}
				}
				if resps, ok := sum["function_responses"].([]map[string]any); ok {
					for _, resp := range resps {
						// Surface isError-ish payloads in text form for debugging.
						b, _ := json.Marshal(resp["response"])
						if bytes.Contains(bytes.ToLower(b), []byte(`"iserror":true`)) ||
							bytes.Contains(bytes.ToLower(b), []byte(`"error"`)) {
							tr.ToolFails = append(tr.ToolFails, string(b))
						}
					}
				}
			}
			if event.UsageMetadata != nil {
				rec["usage"] = event.UsageMetadata
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		_ = enc.Encode(map[string]any{
			"type":        "turn_end",
			"turn":        i,
			"tool_calls":  nToolCalls,
			"tools":       tr.ToolNames,
			"elapsed_ms":  time.Since(turnStart).Milliseconds(),
			"total_ms":    time.Since(start).Milliseconds(),
			"assistant":   strings.Join(tr.Texts, "\n"),
		})
		traces = append(traces, tr)
		fmt.Fprintf(os.Stderr, "[%s] turn %d/%d tools=%v\n", sc.ID, i+1, len(sc.Turns), tr.ToolNames)
	}

	results := evalChecks(sc.Checks, traces)
	allPass := true
	for _, r := range results {
		if !r.Pass {
			allPass = false
		}
		_ = enc.Encode(map[string]any{
			"type":    "check",
			"kind":    r.Kind,
			"pass":    r.Pass,
			"detail":  r.Detail,
		})
		status := "PASS"
		if !r.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(os.Stderr, "[%s] check %s: %s — %s\n", sc.ID, status, r.Kind, r.Detail)
	}

	_ = enc.Encode(map[string]any{
		"type":        "scenario_end",
		"scenario":    sc.ID,
		"elapsed_ms":  time.Since(start).Milliseconds(),
		"all_pass":    allPass,
		"check_count": len(results),
	})
	if !allPass {
		return fmt.Errorf("%s: one or more heuristic checks failed (see log)", sc.ID)
	}
	return nil
}

type checkResult struct {
	Kind   string
	Pass   bool
	Detail string
}

func evalChecks(checks []scenarioCheck, traces []turnTrace) []checkResult {
	var out []checkResult
	allTools := func() []string {
		var names []string
		for _, t := range traces {
			names = append(names, t.ToolNames...)
		}
		return names
	}
	toolsBefore := func(turn int) []string {
		var names []string
		for _, t := range traces {
			if t.Index >= turn {
				break
			}
			names = append(names, t.ToolNames...)
		}
		return names
	}
	allArgs := func() string {
		var b strings.Builder
		for _, t := range traces {
			for _, a := range t.ToolArgs {
				b.WriteString(a)
				b.WriteByte('\n')
			}
		}
		return b.String()
	}
	allText := func() string {
		var b strings.Builder
		for _, t := range traces {
			for _, x := range t.Texts {
				b.WriteString(x)
				b.WriteByte('\n')
			}
		}
		return b.String()
	}
	finalText := func() string {
		if len(traces) == 0 {
			return ""
		}
		return strings.Join(traces[len(traces)-1].Texts, "\n")
	}

	for _, c := range checks {
		switch c.Kind {
		case "never_tools":
			if len(c.Tools) == 0 {
				out = append(out, checkResult{Kind: c.Kind, Pass: true, Detail: "empty tool list (skipped)"})
				continue
			}
			found := intersect(allTools(), c.Tools)
			out = append(out, checkResult{
				Kind:   c.Kind,
				Pass:   len(found) == 0,
				Detail: fmt.Sprintf("forbidden=%v found=%v", c.Tools, found),
			})
		case "no_tools_until_turn":
			found := toolsBefore(c.Turn)
			out = append(out, checkResult{
				Kind:   c.Kind,
				Pass:   len(found) == 0,
				Detail: fmt.Sprintf("tools before turn %d: %v", c.Turn, found),
			})
		case "tools_must_include":
			have := setOf(allTools())
			var missing []string
			for _, t := range c.Tools {
				if !have[t] {
					missing = append(missing, t)
				}
			}
			out = append(out, checkResult{
				Kind:   c.Kind,
				Pass:   len(missing) == 0,
				Detail: fmt.Sprintf("required=%v missing=%v", c.Tools, missing),
			})
		case "tools_after_turn_must_include":
			var after []string
			for _, t := range traces {
				if t.Index >= c.Turn {
					after = append(after, t.ToolNames...)
				}
			}
			have := setOf(after)
			var missing []string
			for _, t := range c.Tools {
				if !have[t] {
					missing = append(missing, t)
				}
			}
			out = append(out, checkResult{
				Kind:   c.Kind,
				Pass:   len(missing) == 0,
				Detail: fmt.Sprintf("from turn %d required=%v missing=%v", c.Turn, c.Tools, missing),
			})
		case "final_text_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			ft := finalText()
			out = append(out, checkResult{
				Kind:   c.Kind,
				Pass:   re.MatchString(ft),
				Detail: fmt.Sprintf("pattern=%q matched=%v (final %d chars)", c.Pattern, re.MatchString(ft), len(ft)),
			})
		case "final_text_not_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			ft := finalText()
			out = append(out, checkResult{
				Kind:   c.Kind,
				Pass:   !re.MatchString(ft),
				Detail: fmt.Sprintf("pattern=%q must_not_match matched=%v", c.Pattern, re.MatchString(ft)),
			})
		case "any_text_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			at := allText()
			out = append(out, checkResult{
				Kind:   c.Kind,
				Pass:   re.MatchString(at),
				Detail: fmt.Sprintf("pattern=%q matched=%v", c.Pattern, re.MatchString(at)),
			})
		case "patch_args_not_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			// Only inspect mutating tool args.
			var mutArgs strings.Builder
			for _, t := range traces {
				for i, name := range t.ToolNames {
					if name == "patchTrip" || name == "replaceDayRoutes" || name == "restoreVersion" {
						if i < len(t.ToolArgs) {
							mutArgs.WriteString(t.ToolArgs[i])
							mutArgs.WriteByte('\n')
						}
					}
				}
			}
			s := mutArgs.String()
			if s == "" {
				s = allArgs() // fallback
			}
			matched := re.MatchString(s)
			out = append(out, checkResult{
				Kind:   c.Kind,
				Pass:   !matched,
				Detail: fmt.Sprintf("pattern=%q matched_in_mutate_args=%v", c.Pattern, matched),
			})
		default:
			out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: "unknown check kind"})
		}
	}
	return out
}

func intersect(have, want []string) []string {
	hs := setOf(have)
	var out []string
	for _, w := range want {
		if hs[w] {
			out = append(out, w)
		}
	}
	return out
}

func setOf(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

// restoreTripVersion calls live MCP restoreVersion directly (setup only; not part of agent turns).
func restoreTripVersion(ctx context.Context, tripID, versionID string) error {
	bearer := strings.TrimSpace(os.Getenv("AGENT_BEARER_TOKEN"))
	if bearer == "" {
		return fmt.Errorf("AGENT_BEARER_TOKEN required for setup restore")
	}
	mcpURL := envOr("TRIPMAP_MCP_URL", defaultMCPURL)
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: bearer})
	client := oauth2.NewClient(ctx, ts)

	sessionID, err := mcpInitialize(ctx, client, mcpURL)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("adk-mt-restore-%d", time.Now().UnixNano())
	// Try common flattened arg shapes (OpenAPI→MCP header/path/body flattening varies).
	candidates := []map[string]any{
		{"id": tripID, "version_id": versionID, "Idempotency-Key": key},
		{"id": tripID, "version_id": versionID, "idempotencyKey": key},
		{"id": tripID, "version_id": versionID, "idempotency_key": key},
	}
	var last string
	for i, argsMap := range candidates {
		args, _ := json.Marshal(argsMap)
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"restoreVersion","arguments":%s}}`, i+2, string(args))
		resp, err := mcpPost(ctx, client, mcpURL, sessionID, body)
		if err != nil {
			return err
		}
		last = resp
		if !mcpToolCallFailed(resp) {
			return nil
		}
	}
	return fmt.Errorf("restoreVersion failed: %s", truncate(last, 500))
}

func mcpToolCallFailed(resp string) bool {
	low := strings.ToLower(resp)
	if strings.Contains(low, `"iserror":true`) {
		return true
	}
	// JSON-RPC error object
	if strings.Contains(low, `"error":{`) && !strings.Contains(low, `"result"`) {
		return true
	}
	return false
}

func mcpInitialize(ctx context.Context, client *http.Client, mcpURL string) (sessionID string, err error) {
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"adk-mt-setup","version":"0"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, strings.NewReader(initBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return "", fmt.Errorf("mcp initialize status=%d body=%s", res.StatusCode, truncate(string(b), 300))
	}
	sessionID = res.Header.Get("Mcp-Session-Id")
	// notifications/initialized (no id)
	_, _ = mcpPost(ctx, client, mcpURL, sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return sessionID, nil
}

func mcpPost(ctx context.Context, client *http.Client, mcpURL, sessionID, body string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	raw := string(b)
	if strings.Contains(raw, "data:") {
		var parts []string
		for _, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(line, "data:") {
				parts = append(parts, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		raw = strings.Join(parts, "\n")
	}
	if res.StatusCode != 200 && res.StatusCode != 202 {
		return raw, fmt.Errorf("mcp post status=%d body=%s", res.StatusCode, truncate(raw, 300))
	}
	return raw, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
