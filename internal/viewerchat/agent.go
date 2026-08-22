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
	"github.com/openai/openai-go/v3/responses"
)

const (
	defaultMaxIter = 24
	defaultModel   = "gpt-5-mini"
	mcpInstructions = "tripmap agent API as MCP tools. Prefer patchTrip with update_day or places.<id>.info; " +
		"do not put enrichment in notes unless the user asks. listTrips then getTrip/getSchema before edits. " +
		"Use listVersions + getVersion to inspect history; restoreVersion only when the user asks to revert. " +
		"Human viewers sign in with Google, then use /me/trips/<id>/."
	viewerChatRules = "OpenAI hosted web_search is for discovery only (find a place, coords, maps links, hours). " +
		"Use at most one focused web_search when you lack lat/lon or maps_url; do not run multiple searches for the same place. " +
		"Never web_search on confirmation turns (user says yes/ok/add it/sounds good, or pastes a maps link) — " +
		"reuse coords/maps_url already in this conversation or from a prior tool result and patch immediately. " +
		"For per-day drive distance or drive time on the current itinerary, call getTrip and read day_stats (drive_dist, drive_min) — " +
		"do not call getTripYAML or estimateDrive for that. Use estimateDrive only for hypothetical routes or places not yet on the trip. " +
		"Ground itinerary facts in tool results (getTrip / getTripYAML). " +
		"To add a stop: if needed, one web_search (include city/region/country), take lat/lon from results; " +
		"then create the place under places (title/lat/lon/type, preferably maps_url) and call upsert_stop " +
		"as a single object with day, list (\"route\" or \"stops\"), and place — never put route/stops under update_day " +
		"(update_day is title/notes/hike/ferry/photo only). " +
		"For overnight or day-endpoint changes use replaceDayRoutes with days as an object keyed by day number " +
		"(not an array); include both day N and N+1 when the overnight moves. " +
		"Never invent latitude/longitude. If you still lack usable coords after one search (or from the user's maps link), ask once — do not keep researching. " +
		"When the user clearly assents to a concrete place or edit already discussed, mutate in that turn; " +
		"do not ask for another confirmation and do not re-research. " +
		"After a successful mutate, confirm in one short sentence in trip terms (day number, place title, where on the day). " +
		"Never mention version_id, revision ids, schema_version, bundle_ok, or other internal API fields to the user."
)

// Config wires the OpenAI Responses agent.
type Config struct {
	APIKey string
	Model  string
	Ops    Ops
	Log    *slog.Logger
	MaxIter int
}

// Agent runs one viewer-chat turn with Eino Responses + in-process tools.
type Agent struct {
	apiKey  string
	model   string
	ops     Ops
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
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	return &Agent{
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   model,
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
		Model:           a.model,
		EnableAutoCache: true,
		Include: []responses.ResponseIncludable{
			responses.ResponseIncludableWebSearchCallActionSources,
		},
	})
	if err != nil {
		return TurnResult{}, fmt.Errorf("responses model: %w", err)
	}

	serverTools := []*agenticopenai.ResponsesServerToolConfig{
		{
			WebSearch: &responses.WebSearchToolParam{
				Type:              responses.WebSearchToolTypeWebSearch,
				SearchContextSize: responses.WebSearchToolSearchContextSizeLow,
			},
		},
	}
	genOpts := []model.Option{
		model.WithTools(infos),
		agenticopenai.WithResponsesServerTools(serverTools),
	}

	instruction := fmt.Sprintf(
		"%s\n\n%s\n\nYou are the in-viewer assistant for trip id %q. "+
			"The trip id is fixed server-side — never switch trips. "+
			"The viewer's current day is %d (1-based). Prefer getTripYAML with scope=day for that day before editing. "+
			"For drive-time or distance questions about existing days, getTrip day_stats is enough — do not load YAML. "+
			"Be concise. Prefer mutate-over-chatter when the user has already chosen.",
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
		stopKeepalive := startSSEKeepalive(ctx, send, "thinking")
		msg, err := am.Generate(ctx, input, genOpts...)
		stopKeepalive()
		latency := time.Since(start).Milliseconds()
		if err != nil {
			tl.with("latency_ms", latency, "model", a.model).Error("model_call", "error", truncateRunes(err.Error(), 300))
			return TurnResult{TripUpdated: tripUpdated}, err
		}
		respID := ""
		if msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.OpenAIExtension != nil {
			respID = msg.ResponseMeta.OpenAIExtension.ID
		}
		calls := functionCalls(msg)
		webSearches := serverToolCallCount(msg)
		tl.with(
			"latency_ms", latency,
			"model", a.model,
			"response_id", respID,
			"tool_calls", len(calls),
			"web_search", webSearches,
		).Info("model_call")

		history = append(history, msg)
		if len(calls) == 0 {
			text = assistantText(msg)
			break
		}
		toolIters++
		toolStatus := toolStatusMessage(calls)
		stopTools := startSSEKeepalive(ctx, send, toolStatus)
		results, err := toolsNode.Invoke(ctx, msg)
		stopTools()
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

func serverToolCallCount(msg *schema.AgenticMessage) int {
	if msg == nil {
		return 0
	}
	n := 0
	for _, b := range msg.ContentBlocks {
		if b != nil && b.ServerToolCall != nil {
			n++
		}
	}
	return n
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

// startSSEKeepalive emits data: status events while a blocking step runs so
// Persona sees activity (comment pings alone are not enough).
func startSSEKeepalive(ctx context.Context, send func(Event) error, status string) func() {
	if send == nil {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		_ = send(Event{Type: "status", Status: status})
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = send(Event{Type: "status", Status: status})
			}
		}
	}()
	return func() { close(stop) }
}

func toolStatusMessage(calls []*schema.FunctionToolCall) string {
	if len(calls) == 0 {
		return "using tools"
	}
	names := make([]string, 0, len(calls))
	seen := make(map[string]bool, len(calls))
	for _, c := range calls {
		if c == nil {
			continue
		}
		name := strings.TrimSpace(c.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		return "using tools"
	}
	return "using " + strings.Join(names, ", ")
}
