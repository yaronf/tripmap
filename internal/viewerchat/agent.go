package viewerchat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultMaxIter = 24
	mcpInstructions = "tripmap agent API as MCP tools. Prefer patchTrip with update_day or places.<id>.info; " +
		"do not put enrichment in notes unless the user asks. listTrips then getTrip/getSchema before edits. " +
		"Use listVersions + getVersion to inspect history; restoreVersion only when the user asks to revert. " +
		"Human viewers sign in with Hellō, then use /me/trips/<id>/."
	viewerChatRules = "Ground itinerary facts in tool results (getTrip / getTripYAML). " +
		"To add a stop: create the place under places (title/lat/lon/type) and call upsert_stop " +
		"with list \"route\" or \"stops\" in the same patchTrip — never put route/stops under update_day " +
		"(update_day is title/notes/hike/ferry/photo only). " +
		"For overnight or day-endpoint changes use replaceDayRoutes. " +
		"If a suggestion is not already in the itinerary, say it is unverified; do not invent precise coordinates."
)

// Config wires the OpenAI Responses agent.
type Config struct {
	APIKey string
	Model  string
	Ops    TripOps
	Log    *slog.Logger
	MaxIter int
}

// Agent runs one viewer-chat turn with Eino Responses + in-process tools.
type Agent struct {
	apiKey  string
	model   string
	ops     TripOps
	log     *slog.Logger
	maxIter int
}

// NewAgent builds an Agent. Ops must be non-nil.
func NewAgent(cfg Config) *Agent {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	max := cfg.MaxIter
	if max <= 0 {
		max = defaultMaxIter
	}
	return &Agent{
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   strings.TrimSpace(cfg.Model),
		ops:     cfg.Ops,
		log:     log,
		maxIter: max,
	}
}

// TurnInput is one Persona chat turn.
type TurnInput struct {
	TripID   string
	UserSub  string
	Messages []ClientMessage
	Day      int
}

// ClientMessage is a normalized chat message.
type ClientMessage struct {
	Role    string
	Content string
}

// TurnResult summarizes one completed turn.
type TurnResult struct {
	Text        string
	TripUpdated bool
}

// Run executes the agent loop and emits SSE text (and relies on caller for trip_updated/done).
func (a *Agent) Run(ctx context.Context, in TurnInput, send func(Event) error, tl turnLogger) (TurnResult, error) {
	if a == nil || a.ops == nil {
		return TurnResult{}, fmt.Errorf("chat agent not configured")
	}
	if a.apiKey == "" {
		return TurnResult{}, fmt.Errorf("OpenAI API key not configured")
	}
	modelName := a.model
	if modelName == "" {
		modelName = "gpt-5-mini"
	}

	tripUpdated := false
	sess := &toolSession{
		ops:         a.ops,
		tripID:      in.TripID,
		viewerDay:   in.Day,
		tripUpdated: &tripUpdated,
		log:         tl,
	}
	tools, err := sess.buildTools()
	if err != nil {
		return TurnResult{}, fmt.Errorf("build tools: %w", err)
	}
	toolsNode, err := compose.NewAgenticToolsNode(ctx, &compose.ToolsNodeConfig{Tools: tools})
	if err != nil {
		return TurnResult{}, fmt.Errorf("tools node: %w", err)
	}
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return TurnResult{}, fmt.Errorf("tool info: %w", err)
		}
		infos = append(infos, info)
	}

	am, err := agenticopenai.NewResponsesModel(ctx, &agenticopenai.ResponsesConfig{
		APIKey:          a.apiKey,
		Model:           modelName,
		EnableAutoCache: true,
	})
	if err != nil {
		return TurnResult{}, fmt.Errorf("responses model: %w", err)
	}

	instruction := fmt.Sprintf(
		"%s\n\n%s\n\nYou are the in-viewer assistant for trip id %q. "+
			"The trip id is fixed server-side — never switch trips. "+
			"The viewer's current day is %d (1-based). Prefer getTripYAML with scope=day for that day before editing. "+
			"Be concise; confirm mutates briefly.",
		mcpInstructions, viewerChatRules, in.TripID, in.Day,
	)

	history := make([]*schema.AgenticMessage, 0, len(in.Messages)+2)
	for _, m := range in.Messages {
		switch strings.ToLower(strings.TrimSpace(m.Role)) {
		case "user":
			history = append(history, schema.UserAgenticMessage(m.Content))
		case "assistant":
			history = append(history, &schema.AgenticMessage{
				Role: schema.AgenticRoleTypeAssistant,
				ContentBlocks: []*schema.ContentBlock{
					schema.NewContentBlock(&schema.AssistantGenText{Text: m.Content}),
				},
			})
		}
	}
	if len(history) == 0 {
		return TurnResult{}, fmt.Errorf("messages required")
	}

	var text string
	toolIters := 0
	for i := 0; i < a.maxIter; i++ {
		input := make([]*schema.AgenticMessage, 0, len(history)+1)
		input = append(input, schema.SystemAgenticMessage(instruction))
		input = append(input, history...)

		start := time.Now()
		msg, err := am.Generate(ctx, input, model.WithTools(infos))
		latency := time.Since(start).Milliseconds()
		if err != nil {
			tl.with("latency_ms", latency, "model", modelName).Error("model_call", "error", truncateRunes(err.Error(), 300))
			return TurnResult{TripUpdated: tripUpdated}, err
		}
		respID := ""
		if msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.OpenAIExtension != nil {
			respID = msg.ResponseMeta.OpenAIExtension.ID
		}
		calls := functionCalls(msg)
		tl.with(
			"latency_ms", latency,
			"model", modelName,
			"response_id", respID,
			"tool_calls", len(calls),
		).Info("model_call")

		history = append(history, msg)
		if len(calls) == 0 {
			text = assistantText(msg)
			break
		}
		toolIters++
		results, err := toolsNode.Invoke(ctx, msg)
		if err != nil {
			return TurnResult{TripUpdated: tripUpdated}, fmt.Errorf("tools: %w", err)
		}
		history = append(history, results...)
		if i == a.maxIter-1 {
			return TurnResult{TripUpdated: tripUpdated}, fmt.Errorf("max iterations (%d) exceeded", a.maxIter)
		}
	}

	tl.with("tool_iters", toolIters, "trip_updated", tripUpdated, "reply_runes", len([]rune(text))).Info("turn_reply")
	if strings.TrimSpace(text) != "" {
		if err := send(Event{Type: "text", Text: text}); err != nil {
			return TurnResult{Text: text, TripUpdated: tripUpdated}, err
		}
	}
	return TurnResult{Text: text, TripUpdated: tripUpdated}, nil
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
