// Command viewerchat-mt runs multi-turn viewer-chat e2e scenarios against an
// in-process tripmapd (Hellō cookie + SSE + tripops tools).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaronf/tripmap/internal/mteval"
)

func main() {
	scenarioPath := flag.String("scenario", "", "path to suite-mt / viewer scenario JSON")
	logPath := flag.String("log", "", "optional JSONL log path")
	day := flag.Int("day", 1, "viewer day context for chat turns")
	flag.Parse()
	if strings.TrimSpace(*scenarioPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/viewerchat-mt --scenario PATH [--log PATH] [--day N]")
		os.Exit(2)
	}

	sc, err := mteval.LoadScenario(*scenarioPath)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sink := newToolSink()
	base := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(sink.Wrap(base)))

	srv, baseURL, err := startLocalServer(ctx)
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	defer srv.Close()

	email, err := chatEmail()
	if err != nil {
		log.Fatal(err)
	}
	cookie, err := mintSessionCookie(srv.cfg.HelloSessionSecret, email)
	if err != nil {
		log.Fatal(err)
	}

	enc, closer, err := openLog(*logPath)
	if err != nil {
		log.Fatal(err)
	}
	defer closer()

	start := time.Now()
	_ = enc.Encode(map[string]any{
		"type": "scenario_start", "framework": "viewerchat", "scenario": sc.ID, "title": sc.Title,
		"hazard": sc.Hazard, "path": *scenarioPath, "turns": len(sc.Turns),
		"model": srv.cfg.OpenAIModel, "base_url": baseURL, "day": *day,
		"setup": sc.Setup, "pass_notes": sc.PassNotes, "ts": time.Now().UTC().Format(time.RFC3339Nano),
	})

	tripID := strings.TrimSpace(sc.Setup.Trip)
	if tripID == "" {
		log.Fatal("scenario setup.trip required")
	}
	if v := strings.TrimSpace(sc.Setup.RestoreVersion); v != "" {
		if err := setupTrip(ctx, baseURL, srv.cfg.AgentBearerToken, tripID, v, srv.mem); err != nil {
			_ = enc.Encode(map[string]any{"type": "setup_error", "error": err.Error()})
			log.Fatalf("setup restore: %v", err)
		}
		_ = enc.Encode(map[string]any{
			"type": "setup_restore", "trip": tripID, "restore_version": v, "mem": srv.mem,
		})
	}

	var (
		history []chatMsg
		traces  []mteval.TurnTrace
	)
	for i, userText := range sc.Turns {
		turnStart := time.Now()
		_ = enc.Encode(map[string]any{"type": "turn_start", "turn": i, "user": userText})
		history = append(history, chatMsg{Role: "user", Content: userText})
		sink.BeginTurn()
		text, err := postChat(ctx, baseURL, tripID, cookie, history, *day)
		tools, args := sink.EndTurn()
		if err != nil {
			_ = enc.Encode(map[string]any{"type": "error", "turn": i, "error": err.Error(), "tools": tools})
			log.Fatalf("turn %d: %v", i, err)
		}
		history = append(history, chatMsg{Role: "assistant", Content: text})
		tr := mteval.TurnTrace{Index: i, User: userText, Texts: []string{text}, ToolNames: tools, ToolArgs: args}
		traces = append(traces, tr)
		_ = enc.Encode(map[string]any{
			"type": "turn_end", "turn": i, "assistant": text, "tools": tools, "tool_args": args,
			"elapsed_ms": time.Since(turnStart).Milliseconds(), "total_ms": time.Since(start).Milliseconds(),
		})
		fmt.Fprintf(os.Stderr, "[%s] turn %d/%d tools=%v\n", sc.ID, i+1, len(sc.Turns), tools)
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
		"all_pass": allPass, "check_count": len(results), "framework": "viewerchat",
	})
	if !allPass {
		log.Fatalf("%s: one or more heuristic checks failed", sc.ID)
	}
	fmt.Fprintf(os.Stderr, "[%s] all checks passed\n", sc.ID)
}

func openLog(path string) (*json.Encoder, func(), error) {
	if strings.TrimSpace(path) == "" {
		enc := json.NewEncoder(os.Stdout)
		return enc, func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return json.NewEncoder(f), func() { _ = f.Close() }, nil
}

func chatEmail() (string, error) {
	if e := strings.TrimSpace(os.Getenv("CHAT_EMAIL")); e != "" {
		return e, nil
	}
	// First chat=yes row from config/users.csv (same heuristic as smoke-chat).
	b, err := os.ReadFile("config/users.csv")
	if err != nil {
		return "", fmt.Errorf("CHAT_EMAIL or config/users.csv required: %w", err)
	}
	for i, line := range strings.Split(string(b), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		email := strings.TrimSpace(parts[0])
		chat := strings.ToLower(strings.TrimSpace(parts[2]))
		switch chat {
		case "yes", "true", "1", "y", "chat":
			if email != "" {
				return email, nil
			}
		}
	}
	return "", fmt.Errorf("set CHAT_EMAIL or add a chat=yes row to config/users.csv")
}
