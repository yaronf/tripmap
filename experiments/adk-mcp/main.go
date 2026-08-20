// Tripmap ADK Go + MCP experiment: one plain LLM agent, live MCP tools only.
// See docs/adk-go-mcp-experiment.md and README.md in this directory.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	openaimodel "google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/mcptoolset"
)

// Copied from tripmapd MCP handler instructions (internal/httpserver/server.go),
// with one ADK-only tweak: /me/trips/{id}/ → /me/trips/<id>/ so ADK does not
// treat {id} as a missing session-state placeholder (see instructionutil).
const mcpServerInstructions = "tripmap agent API as MCP tools. Prefer patchTrip with update_day or places.<id>.info; " +
	"do not put enrichment in notes unless the user asks. listTrips then getTrip/getSchema before edits. " +
	"Use listVersions + getVersion to inspect history; restoreVersion only when the user asks to revert. " +
	"Human viewers sign in with Hellō, then use /me/trips/<id>/."

const (
	defaultMCPURL = "https://tripmap.sheffer.org/mcp"
	defaultModel  = "gpt-4o"
	appName       = "tripmap-adk-mcp-experiment"
)

func main() {
	prompt := flag.String("prompt", "", "one-shot user prompt (writes JSONL events; skips ADK launcher)")
	logPath := flag.String("log", "", "JSONL event log path for --prompt (default: stdout)")
	userID := flag.String("user", "experiment", "session user id for --prompt")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	a, err := newAgent(ctx)
	if err != nil {
		log.Fatal(err)
	}

	if strings.TrimSpace(*prompt) != "" {
		if err := runPrompt(ctx, a, *userID, *prompt, *logPath); err != nil {
			log.Fatal(err)
		}
		return
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(a),
	}
	l := full.NewLauncher()
	args := flag.Args()
	if len(args) == 0 {
		args = []string{"console"}
	}
	if err := l.Execute(ctx, config, args); err != nil {
		log.Fatalf("run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

func newAgent(ctx context.Context) (agent.Agent, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if apiKey == "" && baseURL == "" {
		return nil, fmt.Errorf("set OPENAI_API_KEY (or OPENAI_BASE_URL for a compatible endpoint)")
	}

	modelName := envOr("OPENAI_MODEL", defaultModel)
	llm, err := openaimodel.NewModel(ctx, modelName, &openaimodel.ClientConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("openai model: %w", err)
	}

	bearer := strings.TrimSpace(os.Getenv("AGENT_BEARER_TOKEN"))
	if bearer == "" {
		return nil, fmt.Errorf("set AGENT_BEARER_TOKEN (raw token, no Bearer prefix)")
	}
	mcpURL := envOr("TRIPMAP_MCP_URL", defaultMCPURL)

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: bearer})
	httpClient := oauth2.NewClient(ctx, ts)
	// Prefer request/response only; tripmap MCP smoke uses POST tools/list without a GET SSE stream.
	transport := &mcp.StreamableClientTransport{
		Endpoint:             mcpURL,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}

	mcpTools, err := mcptoolset.New(mcptoolset.Config{Transport: transport})
	if err != nil {
		return nil, fmt.Errorf("mcp toolset: %w", err)
	}

	return llmagent.New(llmagent.Config{
		Name:        "tripmap_mcp_agent",
		Model:       llm,
		Description: "Tripmap itinerary agent via live MCP tools only.",
		Instruction: mcpServerInstructions,
		Toolsets:    []tool.Toolset{mcpTools},
	})
}

func runPrompt(ctx context.Context, a agent.Agent, userID, prompt, logPath string) error {
	start := time.Now()
	ss := session.InMemoryService()
	created, err := ss.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  userID,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          a,
		SessionService: ss,
	})
	if err != nil {
		return fmt.Errorf("runner: %w", err)
	}

	out := os.Stdout
	if logPath != "" {
		f, err := os.Create(logPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	meta := map[string]any{
		"type":       "run_start",
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
		"model":      envOr("OPENAI_MODEL", defaultModel),
		"mcp_url":    envOr("TRIPMAP_MCP_URL", defaultMCPURL),
		"prompt":     prompt,
		"user_id":    userID,
		"session_id": created.Session.ID(),
	}
	if err := enc.Encode(meta); err != nil {
		return err
	}

	msg := genai.NewContentFromText(prompt, genai.RoleUser)
	var nEvents, nToolCalls int
	for event, err := range r.Run(ctx, userID, created.Session.ID(), msg, agent.RunConfig{}) {
		rec := map[string]any{
			"type":      "event",
			"ts":        time.Now().UTC().Format(time.RFC3339Nano),
			"elapsed_ms": time.Since(start).Milliseconds(),
		}
		if err != nil {
			rec["error"] = err.Error()
			_ = enc.Encode(rec)
			return err
		}
		nEvents++
		rec["event_id"] = event.ID
		rec["author"] = event.Author
		rec["invocation_id"] = event.InvocationID
		if event.ErrorCode != "" || event.ErrorMessage != "" {
			rec["error_code"] = event.ErrorCode
			rec["error_message"] = event.ErrorMessage
		}
		if event.Content != nil {
			rec["content"] = summarizeContent(event.Content, &nToolCalls)
		}
		if event.UsageMetadata != nil {
			rec["usage"] = event.UsageMetadata
		}
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}

	return enc.Encode(map[string]any{
		"type":         "run_end",
		"ts":           time.Now().UTC().Format(time.RFC3339Nano),
		"elapsed_ms":   time.Since(start).Milliseconds(),
		"event_count":  nEvents,
		"tool_calls":   nToolCalls,
	})
}

func summarizeContent(c *genai.Content, toolCalls *int) map[string]any {
	out := map[string]any{"role": c.Role}
	var texts []string
	var calls []map[string]any
	var responses []map[string]any
	for _, p := range c.Parts {
		if p == nil {
			continue
		}
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
		if p.FunctionCall != nil {
			*toolCalls++
			calls = append(calls, map[string]any{
				"id":   p.FunctionCall.ID,
				"name": p.FunctionCall.Name,
				"args": p.FunctionCall.Args,
			})
		}
		if p.FunctionResponse != nil {
			responses = append(responses, map[string]any{
				"id":       p.FunctionResponse.ID,
				"name":     p.FunctionResponse.Name,
				"response": p.FunctionResponse.Response,
			})
		}
	}
	if len(texts) > 0 {
		out["text"] = strings.Join(texts, "\n")
	}
	if len(calls) > 0 {
		out["function_calls"] = calls
	}
	if len(responses) > 0 {
		out["function_responses"] = responses
	}
	return out
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}