package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

const (
	maxToolIterations = 8
	maxYAMLToolBytes  = 48 * 1024
)

// Config configures the chat agent.
type Config struct {
	APIKey string
	Model  string
	Ops    TripOps
}

type respondFunc func(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, error)

// Agent runs the OpenAI Responses tool loop for one viewer chat turn.
type Agent struct {
	client  openai.Client
	model   string
	ops     TripOps
	respond respondFunc // optional; defaults to OpenAI Responses API
}

// NewAgent builds an Agent. APIKey must be non-empty.
func NewAgent(cfg Config) *Agent {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = string(openai.ChatModelGPT4oMini)
	}
	a := &Agent{
		client: openai.NewClient(option.WithAPIKey(cfg.APIKey)),
		model:  model,
		ops:    cfg.Ops,
	}
	a.respond = a.openaiRespond
	return a
}

func (a *Agent) openaiRespond(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, error) {
	return a.client.Responses.New(ctx, params)
}

// TurnInput is one chat request scoped to a trip.
type TurnInput struct {
	TripID   string
	Messages []ClientMessage
	Day      int // 1-based current day from the viewer; 0 if unknown
}

// ClientMessage is a Persona transcript message.
type ClientMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type turnResult struct {
	TripUpdated bool
}

func (a *Agent) run(ctx context.Context, in TurnInput, emit func(Event) error) (turnResult, error) {
	card, err := a.ops.Summary(ctx, in.TripID)
	if err != nil {
		return turnResult{}, err
	}

	inputItems, err := buildInputItems(in.Messages)
	if err != nil {
		return turnResult{}, err
	}
	if in.Day > 0 {
		title := dayTitle(card, in.Day)
		note := fmt.Sprintf("Viewer context: the user is looking at day %d", in.Day)
		if title != "" {
			note += " (" + title + ")"
		}
		note += `. Phrases like "this day" / "today" / "here" refer to that day number only.`
		inputItems = append([]responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfMessage(note, responses.EasyInputMessageRoleDeveloper),
		}, inputItems...)
	}

	respond := a.respond
	if respond == nil {
		respond = a.openaiRespond
	}

	tools := chatTools()
	params := responses.ResponseNewParams{
		Model:        a.model,
		Instructions: openai.String(buildSystemPrompt(card, in.Day)),
		Tools:        tools,
		Store:        openai.Bool(true),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: inputItems,
		},
	}

	var result turnResult
	for iter := 0; iter < maxToolIterations; iter++ {
		resp, err := respond(ctx, params)
		if err != nil {
			return result, fmt.Errorf("openai: %w", err)
		}

		fnCalls := functionCalls(resp)
		if len(fnCalls) == 0 {
			text := cleanAssistantText(resp.OutputText())
			if text != "" {
				if err := emit(Event{Type: "text", Text: text}); err != nil {
					return result, err
				}
			}
			return result, nil
		}

		outputs := make([]responses.ResponseInputItemUnionParam, 0, len(fnCalls))
		for _, fc := range fnCalls {
			out, patched, callErr := a.execTool(ctx, in.TripID, fc.Name, fc.Arguments)
			if patched {
				result.TripUpdated = true
			}
			content := out
			if callErr != nil {
				content = fmt.Sprintf(`{"error":%q}`, callErr.Error())
			}
			outputs = append(outputs, responses.ResponseInputItemUnionParam{
				OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: fc.CallID,
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfString: openai.String(content),
					},
				},
			})
		}

		params = responses.ResponseNewParams{
			Model:              a.model,
			PreviousResponseID: openai.String(resp.ID),
			Tools:              tools,
			Store:              openai.Bool(true),
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: outputs,
			},
		}
	}
	return result, fmt.Errorf("tool iteration limit reached")
}

func buildInputItems(msgs []ClientMessage) ([]responses.ResponseInputItemUnionParam, error) {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(msgs))
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		switch role {
		case "user":
			items = append(items, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser))
		case "assistant":
			items = append(items, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleAssistant))
		case "system", "developer":
			// Server owns instructions; ignore client system/developer turns.
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no user messages")
	}
	return items, nil
}

func functionCalls(resp *responses.Response) []responses.ResponseFunctionToolCall {
	if resp == nil {
		return nil
	}
	var out []responses.ResponseFunctionToolCall
	for _, item := range resp.Output {
		if item.Type == "function_call" {
			out = append(out, item.AsFunctionCall())
		}
	}
	return out
}

func buildSystemPrompt(card TripCard, day int) string {
	var b strings.Builder
	b.WriteString(baseSystemPrompt())
	b.WriteString("Trip context (JSON):\n")
	raw, _ := json.Marshal(card)
	b.Write(raw)
	if day > 0 {
		title := dayTitle(card, day)
		fmt.Fprintf(&b, "\nCURRENT VIEWER DAY: %d", day)
		if title != "" {
			fmt.Fprintf(&b, " — %s", title)
		}
		b.WriteString(".\n")
		b.WriteString("When the user says \"this day\", \"today\", \"the current day\", \"here\", or similar, they ALWAYS mean this CURRENT VIEWER DAY (the day shown in the viewer), not day 1 and not a day inferred from chat history. Call get_day on that day number before editing it.\n")
	} else {
		b.WriteString("\nCURRENT VIEWER DAY: unknown (client did not send day). Ask which day if needed.\n")
	}
	return b.String()
}

func dayTitle(card TripCard, day int) string {
	prefix := fmt.Sprintf("%d:", day)
	for _, t := range card.DayTitles {
		if strings.HasPrefix(t, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(t, prefix))
		}
	}
	return ""
}
