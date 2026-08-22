// Command viewerchat-suite runs viewer-chat e2e scenarios (MT context suite +
// S one-shot sophistication suite) against in-process tripmapd.
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
	"sort"
	"strings"
	"time"

	"github.com/yaronf/tripmap/internal/mteval"
)

func main() {
	scenarioPath := flag.String("scenario", "", "path to one scenario JSON")
	scenarioDir := flag.String("dir", "", "run all *.json scenarios in this directory")
	logPath := flag.String("log", "", "JSONL log path (single scenario; default stdout). With --dir, logs go under <dir>/../runs/<id>.jsonl unless set as a directory")
	day := flag.Int("day", 1, "viewer day context for chat turns")
	flag.Parse()

	paths, err := scenarioPaths(*scenarioPath, *scenarioDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/viewerchat-suite --scenario PATH | --dir DIR [--log PATH] [--day N]")
		os.Exit(2)
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

	var failed []string
	for _, path := range paths {
		sc, err := mteval.LoadScenario(path)
		if err != nil {
			log.Fatalf("load %s: %v", path, err)
		}
		lp := *logPath
		if *scenarioDir != "" {
			lp = filepath.Join(filepath.Dir(*scenarioDir), "runs", sc.ID+".jsonl")
			if strings.TrimSpace(*logPath) != "" {
				// Treat --log as a directory when running a suite.
				lp = filepath.Join(*logPath, sc.ID+".jsonl")
			}
		}
		ok, err := runScenario(ctx, srv, baseURL, cookie, sink, sc, path, lp, *day)
		if err != nil {
			log.Printf("%s: %v", sc.ID, err)
			failed = append(failed, sc.ID)
			continue
		}
		if !ok {
			failed = append(failed, sc.ID)
		}
	}
	if len(failed) > 0 {
		log.Fatalf("failed: %s", strings.Join(failed, ", "))
	}
}

func scenarioPaths(one, dir string) ([]string, error) {
	one = strings.TrimSpace(one)
	dir = strings.TrimSpace(dir)
	switch {
	case one != "" && dir != "":
		return nil, fmt.Errorf("use --scenario or --dir, not both")
	case one != "":
		return []string{one}, nil
	case dir != "":
		matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		if len(matches) == 0 {
			return nil, fmt.Errorf("no *.json in %s", dir)
		}
		return matches, nil
	default:
		return nil, nil
	}
}

func runScenario(
	ctx context.Context,
	srv *localServer,
	baseURL, cookie string,
	sink *toolSink,
	sc *mteval.Scenario,
	scenarioPath, logPath string,
	day int,
) (allPass bool, err error) {
	enc, closer, err := openLog(logPath)
	if err != nil {
		return false, err
	}
	defer closer()

	start := time.Now()
	_ = enc.Encode(map[string]any{
		"type": "scenario_start", "framework": "viewerchat", "scenario": sc.ID, "title": sc.Title,
		"hazard": sc.Hazard, "path": scenarioPath, "turns": len(sc.Turns),
		"model": srv.cfg.OpenAIModel, "base_url": baseURL, "day": day,
		"setup": sc.Setup, "pass_notes": sc.PassNotes, "ts": time.Now().UTC().Format(time.RFC3339Nano),
	})

	tripID := strings.TrimSpace(sc.Setup.Trip)
	if tripID == "" {
		return false, fmt.Errorf("setup.trip required")
	}
	if v := strings.TrimSpace(sc.Setup.RestoreVersion); v != "" {
		if err := setupTrip(ctx, baseURL, srv.cfg.AgentBearerToken, tripID, v, srv.mem); err != nil {
			_ = enc.Encode(map[string]any{"type": "setup_error", "error": err.Error()})
			return false, fmt.Errorf("setup restore: %w", err)
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
		text, err := postChat(ctx, baseURL, tripID, cookie, history, day)
		tools, args := sink.EndTurn()
		if err != nil {
			_ = enc.Encode(map[string]any{"type": "error", "turn": i, "error": err.Error(), "tools": tools})
			return false, fmt.Errorf("turn %d: %w", i, err)
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
	allPass = mteval.AllPassed(results)
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
	if allPass {
		fmt.Fprintf(os.Stderr, "[%s] all checks passed\n", sc.ID)
	} else {
		fmt.Fprintf(os.Stderr, "[%s] one or more heuristic checks failed\n", sc.ID)
	}
	return allPass, nil
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
