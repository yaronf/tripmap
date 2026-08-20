package main

import (
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
)

type scenarioFile struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Hazard    string          `json:"hazard"`
	Setup     scenarioSetup   `json:"setup"`
	Turns     []string        `json:"turns"`
	Checks    []scenarioCheck `json:"checks"`
	PassNotes string          `json:"pass_notes"`
}

type scenarioSetup struct {
	Trip           string `json:"trip"`
	RestoreVersion string `json:"restore_version"`
}

type scenarioCheck struct {
	Kind    string   `json:"kind"`
	Turn    int      `json:"turn"`
	Tools   []string `json:"tools"`
	Pattern string   `json:"pattern"`
}

type turnTrace struct {
	Index     int
	User      string
	Texts     []string
	ToolNames []string
	ToolArgs  []string
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

func runScenario(ctx context.Context, agent *tripmapAgent, scenarioPath, logPath string) error {
	sc, err := loadScenario(scenarioPath)
	if err != nil {
		return err
	}
	start := time.Now()
	enc, closer, err := openLog(logPath)
	if err != nil {
		return err
	}
	defer closer()

	_ = enc.Encode(map[string]any{
		"type": "scenario_start", "framework": "eino", "scenario": sc.ID, "title": sc.Title,
		"hazard": sc.Hazard, "path": scenarioPath, "turns": len(sc.Turns),
		"model": envOr("OPENAI_MODEL", defaultModel), "mcp_url": envOr("TRIPMAP_MCP_URL", defaultMCPURL),
		"setup": sc.Setup, "pass_notes": sc.PassNotes, "ts": time.Now().UTC().Format(time.RFC3339Nano),
	})

	if sc.Setup.Trip != "" && sc.Setup.RestoreVersion != "" {
		if err := restoreTripVersion(ctx, sc.Setup.Trip, sc.Setup.RestoreVersion); err != nil {
			_ = enc.Encode(map[string]any{"type": "setup_error", "error": err.Error()})
			return fmt.Errorf("setup restore: %w", err)
		}
		_ = enc.Encode(map[string]any{
			"type": "setup_restore", "trip": sc.Setup.Trip, "restore_version": sc.Setup.RestoreVersion,
		})
	}

	agent.Reset()
	var traces []turnTrace
	for i, userText := range sc.Turns {
		turnStart := time.Now()
		_ = enc.Encode(map[string]any{"type": "turn_start", "turn": i, "user": userText})
		res, err := agent.Turn(ctx, userText)
		if err != nil {
			_ = enc.Encode(map[string]any{"type": "error", "turn": i, "error": err.Error()})
			return fmt.Errorf("turn %d: %w", i, err)
		}
		tr := turnTrace{Index: i, User: userText, Texts: []string{res.Text}, ToolNames: res.ToolNames, ToolArgs: res.ToolArgs}
		traces = append(traces, tr)
		_ = enc.Encode(map[string]any{
			"type": "turn_end", "turn": i, "assistant": res.Text, "tools": res.ToolNames,
			"tool_args": res.ToolArgs, "elapsed_ms": time.Since(turnStart).Milliseconds(),
			"total_ms": time.Since(start).Milliseconds(),
		})
		fmt.Fprintf(os.Stderr, "[%s] turn %d/%d tools=%v\n", sc.ID, i+1, len(sc.Turns), res.ToolNames)
	}

	results := evalChecks(sc.Checks, traces)
	allPass := true
	for _, r := range results {
		if !r.Pass {
			allPass = false
		}
		_ = enc.Encode(map[string]any{"type": "check", "kind": r.Kind, "pass": r.Pass, "detail": r.Detail})
		status := "PASS"
		if !r.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(os.Stderr, "[%s] check %s: %s — %s\n", sc.ID, status, r.Kind, r.Detail)
	}
	_ = enc.Encode(map[string]any{
		"type": "scenario_end", "scenario": sc.ID, "elapsed_ms": time.Since(start).Milliseconds(),
		"all_pass": allPass, "check_count": len(results), "framework": "eino",
	})
	if !allPass {
		return fmt.Errorf("%s: one or more heuristic checks failed", sc.ID)
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
	setOf := func(ss []string) map[string]bool {
		m := make(map[string]bool, len(ss))
		for _, s := range ss {
			m[s] = true
		}
		return m
	}
	intersect := func(have, want []string) []string {
		hs := setOf(have)
		var o []string
		for _, w := range want {
			if hs[w] {
				o = append(o, w)
			}
		}
		return o
	}

	for _, c := range checks {
		switch c.Kind {
		case "never_tools":
			if len(c.Tools) == 0 {
				out = append(out, checkResult{Kind: c.Kind, Pass: true, Detail: "empty tool list (skipped)"})
				continue
			}
			found := intersect(allTools(), c.Tools)
			out = append(out, checkResult{Kind: c.Kind, Pass: len(found) == 0,
				Detail: fmt.Sprintf("forbidden=%v found=%v", c.Tools, found)})
		case "no_tools_until_turn":
			found := toolsBefore(c.Turn)
			out = append(out, checkResult{Kind: c.Kind, Pass: len(found) == 0,
				Detail: fmt.Sprintf("tools before turn %d: %v", c.Turn, found)})
		case "tools_must_include":
			have := setOf(allTools())
			var missing []string
			for _, t := range c.Tools {
				if !have[t] {
					missing = append(missing, t)
				}
			}
			out = append(out, checkResult{Kind: c.Kind, Pass: len(missing) == 0,
				Detail: fmt.Sprintf("required=%v missing=%v", c.Tools, missing)})
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
			out = append(out, checkResult{Kind: c.Kind, Pass: len(missing) == 0,
				Detail: fmt.Sprintf("from turn %d required=%v missing=%v", c.Turn, c.Tools, missing)})
		case "final_text_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			ft := finalText()
			out = append(out, checkResult{Kind: c.Kind, Pass: re.MatchString(ft),
				Detail: fmt.Sprintf("pattern=%q matched=%v", c.Pattern, re.MatchString(ft))})
		case "final_text_not_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			ft := finalText()
			out = append(out, checkResult{Kind: c.Kind, Pass: !re.MatchString(ft),
				Detail: fmt.Sprintf("pattern=%q matched=%v", c.Pattern, re.MatchString(ft))})
		case "any_text_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			at := allText()
			out = append(out, checkResult{Kind: c.Kind, Pass: re.MatchString(at),
				Detail: fmt.Sprintf("pattern=%q matched=%v", c.Pattern, re.MatchString(at))})
		case "patch_args_not_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
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
			matched := re.MatchString(s)
			out = append(out, checkResult{Kind: c.Kind, Pass: !matched,
				Detail: fmt.Sprintf("pattern=%q matched_in_mutate_args=%v", c.Pattern, matched)})
		default:
			out = append(out, checkResult{Kind: c.Kind, Pass: false, Detail: "unknown check kind"})
		}
	}
	return out
}

func restoreTripVersion(ctx context.Context, tripID, versionID string) error {
	bearer := strings.TrimSpace(os.Getenv("AGENT_BEARER_TOKEN"))
	if bearer == "" {
		return fmt.Errorf("AGENT_BEARER_TOKEN required")
	}
	mcpURL := envOr("TRIPMAP_MCP_URL", defaultMCPURL)
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: bearer}))
	sessionID, err := mcpInitialize(ctx, client, mcpURL)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("eino-mt-restore-%d", time.Now().UnixNano())
	for i, argsMap := range []map[string]any{
		{"id": tripID, "version_id": versionID, "Idempotency-Key": key},
		{"id": tripID, "version_id": versionID, "idempotency_key": key},
	} {
		args, _ := json.Marshal(argsMap)
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"restoreVersion","arguments":%s}}`, i+2, args)
		resp, err := mcpPost(ctx, client, mcpURL, sessionID, body)
		if err != nil {
			return err
		}
		if !mcpToolCallFailed(resp) {
			return nil
		}
	}
	return fmt.Errorf("restoreVersion failed")
}

func mcpToolCallFailed(resp string) bool {
	low := strings.ToLower(resp)
	return strings.Contains(low, `"iserror":true`) || (strings.Contains(low, `"error":{`) && !strings.Contains(low, `"result"`))
}

func mcpInitialize(ctx context.Context, client *http.Client, mcpURL string) (string, error) {
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"eino-mt-setup","version":"0"}}}`
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
	sessionID := res.Header.Get("Mcp-Session-Id")
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
