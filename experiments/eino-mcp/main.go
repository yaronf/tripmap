// Tripmap Eino + MCP experiment: AgenticModel Responses + live MCP tools.
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

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/oauth2"
)

// Same instruction text as ADK experiment / tripmapd MCP instructions
// (with <id> not {id} — harmless for Eino, keeps prompt parity).
const mcpServerInstructions = "tripmap agent API as MCP tools. Prefer patchTrip with update_day or places.<id>.info; " +
	"do not put enrichment in notes unless the user asks. listTrips then getTrip/getSchema before edits. " +
	"Use listVersions + getVersion to inspect history; restoreVersion only when the user asks to revert. " +
	"Human viewers sign in with Hellō, then use /me/trips/<id>/."

const (
	defaultMCPURL = "https://tripmap.sheffer.org/mcp"
	defaultModel  = "gpt-5-mini"
)

func main() {
	prompt := flag.String("prompt", "", "one-shot user prompt (JSONL log)")
	scenario := flag.String("scenario", "", "multi-turn scenario JSON (reuse adk-mcp suite-mt)")
	logPath := flag.String("log", "", "JSONL log path (default stdout)")
	noCache := flag.Bool("no-cache", false, "disable EnableAutoCache (full client history)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	agent, cleanup, err := newTripmapAgent(ctx, !*noCache)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	switch {
	case strings.TrimSpace(*scenario) != "":
		if err := runScenario(ctx, agent, *scenario, *logPath); err != nil {
			log.Fatal(err)
		}
	case strings.TrimSpace(*prompt) != "":
		if err := runPrompt(ctx, agent, *prompt, *logPath); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("use --prompt or --scenario")
	}
}

type tripmapAgent struct {
	model       model.AgenticModel
	toolsNode   *compose.AgenticToolsNode
	toolInfos   []*schema.ToolInfo
	instruction string
	history     []*schema.AgenticMessage
	maxIter     int
}

func newTripmapAgent(ctx context.Context, autoCache bool) (*tripmapAgent, func(), error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil, nil, fmt.Errorf("set OPENAI_API_KEY")
	}
	bearer := strings.TrimSpace(os.Getenv("AGENT_BEARER_TOKEN"))
	if bearer == "" {
		return nil, nil, fmt.Errorf("set AGENT_BEARER_TOKEN")
	}
	mcpURL := envOr("TRIPMAP_MCP_URL", defaultMCPURL)
	modelName := envOr("OPENAI_MODEL", defaultModel)

	am, err := agenticopenai.NewResponsesModel(ctx, &agenticopenai.ResponsesConfig{
		APIKey:          apiKey,
		BaseURL:         os.Getenv("OPENAI_BASE_URL"),
		Model:           modelName,
		EnableAutoCache: autoCache,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("responses model: %w", err)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: bearer})
	httpClient := oauth2.NewClient(ctx, ts)
	cli, err := client.NewStreamableHttpClient(mcpURL,
		transport.WithHTTPBasicClient(httpClient),
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + bearer,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("mcp client: %w", err)
	}
	if err := cli.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("mcp start: %w", err)
	}
	if _, err := cli.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "tripmap-eino-mcp", Version: "0"},
		},
	}); err != nil {
		_ = cli.Close()
		return nil, nil, fmt.Errorf("mcp initialize: %w", err)
	}

	baseTools, err := mcpp.GetTools(ctx, &mcpp.Config{Cli: cli})
	if err != nil {
		_ = cli.Close()
		return nil, nil, fmt.Errorf("mcp get tools: %w", err)
	}
	toolsNode, err := compose.NewAgenticToolsNode(ctx, &compose.ToolsNodeConfig{Tools: baseTools})
	if err != nil {
		_ = cli.Close()
		return nil, nil, fmt.Errorf("tools node: %w", err)
	}
	infos := make([]*schema.ToolInfo, 0, len(baseTools))
	for _, t := range baseTools {
		info, err := t.Info(ctx)
		if err != nil {
			_ = cli.Close()
			return nil, nil, fmt.Errorf("tool info: %w", err)
		}
		infos = append(infos, info)
	}

	cleanup := func() { _ = cli.Close() }
	return &tripmapAgent{
		model:       am,
		toolsNode:   toolsNode,
		toolInfos:   infos,
		instruction: mcpServerInstructions,
		maxIter:     24,
	}, cleanup, nil
}

type turnResult struct {
	Text      string
	ToolNames []string
	ToolArgs  []string
}

func (a *tripmapAgent) Reset() {
	a.history = nil
}

func (a *tripmapAgent) Turn(ctx context.Context, userText string) (*turnResult, error) {
	a.history = append(a.history, schema.UserAgenticMessage(userText))
	out := &turnResult{}

	for i := 0; i < a.maxIter; i++ {
		input := make([]*schema.AgenticMessage, 0, len(a.history)+1)
		if a.instruction != "" {
			input = append(input, schema.SystemAgenticMessage(a.instruction))
		}
		input = append(input, a.history...)

		msg, err := a.model.Generate(ctx, input, model.WithTools(a.toolInfos))
		if err != nil {
			return out, err
		}
		a.history = append(a.history, msg)

		calls := functionCalls(msg)
		if len(calls) == 0 {
			out.Text = assistantText(msg)
			return out, nil
		}
		for _, c := range calls {
			out.ToolNames = append(out.ToolNames, c.Name)
			out.ToolArgs = append(out.ToolArgs, c.Arguments)
		}
		results, err := a.toolsNode.Invoke(ctx, msg)
		if err != nil {
			return out, fmt.Errorf("tools: %w", err)
		}
		a.history = append(a.history, results...)
	}
	return out, fmt.Errorf("max iterations (%d) exceeded", a.maxIter)
}

func functionCalls(msg *schema.AgenticMessage) []*schema.FunctionToolCall {
	if msg == nil {
		return nil
	}
	var out []*schema.FunctionToolCall
	for _, b := range msg.ContentBlocks {
		if b != nil && b.FunctionToolCall != nil {
			out = append(out, b.FunctionToolCall)
		}
	}
	return out
}

func assistantText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range msg.ContentBlocks {
		if block != nil && block.AssistantGenText != nil {
			b.WriteString(block.AssistantGenText.Text)
		}
	}
	return b.String()
}

func runPrompt(ctx context.Context, agent *tripmapAgent, prompt, logPath string) error {
	start := time.Now()
	enc, closer, err := openLog(logPath)
	if err != nil {
		return err
	}
	defer closer()

	_ = enc.Encode(map[string]any{
		"type": "run_start", "framework": "eino", "model": envOr("OPENAI_MODEL", defaultModel),
		"prompt": prompt, "ts": time.Now().UTC().Format(time.RFC3339Nano),
	})
	agent.Reset()
	res, err := agent.Turn(ctx, prompt)
	if err != nil {
		_ = enc.Encode(map[string]any{"type": "error", "error": err.Error()})
		return err
	}
	_ = enc.Encode(map[string]any{
		"type": "turn_end", "assistant": res.Text, "tools": res.ToolNames, "tool_args": res.ToolArgs,
		"elapsed_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

func openLog(logPath string) (*json.Encoder, func(), error) {
	out := os.Stdout
	closer := func() {}
	if logPath != "" {
		if err := os.MkdirAll(dirOf(logPath), 0o755); err != nil {
			return nil, nil, err
		}
		f, err := os.Create(logPath)
		if err != nil {
			return nil, nil, err
		}
		out = f
		closer = func() { _ = f.Close() }
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc, closer, nil
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}
