package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yaronf/tripmap/internal/mteval"
	"golang.org/x/oauth2"
)

func runScenario(ctx context.Context, agent *tripmapAgent, scenarioPath, logPath string) error {
	sc, err := mteval.LoadScenario(scenarioPath)
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
	var traces []mteval.TurnTrace
	for i, userText := range sc.Turns {
		turnStart := time.Now()
		_ = enc.Encode(map[string]any{"type": "turn_start", "turn": i, "user": userText})
		res, err := agent.Turn(ctx, userText)
		if err != nil {
			_ = enc.Encode(map[string]any{"type": "error", "turn": i, "error": err.Error()})
			return fmt.Errorf("turn %d: %w", i, err)
		}
		tr := mteval.TurnTrace{Index: i, User: userText, Texts: []string{res.Text}, ToolNames: res.ToolNames, ToolArgs: res.ToolArgs}
		traces = append(traces, tr)
		_ = enc.Encode(map[string]any{
			"type": "turn_end", "turn": i, "assistant": res.Text, "tools": res.ToolNames,
			"tool_args": res.ToolArgs, "elapsed_ms": time.Since(turnStart).Milliseconds(),
			"total_ms": time.Since(start).Milliseconds(),
		})
		fmt.Fprintf(os.Stderr, "[%s] turn %d/%d tools=%v\n", sc.ID, i+1, len(sc.Turns), res.ToolNames)
	}

	results := mteval.EvalChecks(sc.Checks, traces)
	allPass := mteval.AllPassed(results)
	for _, r := range results {
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
