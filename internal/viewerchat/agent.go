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
	maxToolIterations  = 10
	maxRepairRounds    = 2
	maxYAMLToolBytes   = 48 * 1024
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
		model = string(openai.ChatModelGPT4o)
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

// TurnInput is one chat request scoped to a trip and signed-in user.
type TurnInput struct {
	TripID   string
	UserSub  string // Hellō subject; required for preference/learning tools
	Messages []ClientMessage
	Day      int // 1-based current day from the viewer; 0 if unknown
	// FeedbackDown is set when the client reports a prior thumbs-down to act on.
	FeedbackDown *FeedbackDown
}

// FeedbackDown carries the last exchange the user disliked.
type FeedbackDown struct {
	UserText      string
	AssistantText string
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

	prefs := []Preference{}
	learnings := []Learning{}
	if strings.TrimSpace(in.UserSub) != "" {
		if list, err := a.ops.ListPreferences(ctx, in.UserSub); err == nil {
			prefs = list
		}
		if list, err := a.ops.ListLearnings(ctx, in.UserSub); err == nil {
			learnings = list
		}
	}

	var fragment *TripFragment
	if in.Day > 0 {
		if body, err := a.ops.GetYAML(ctx, in.TripID); err == nil {
			if frag, err := BuildTripFragment(body, in.Day); err == nil && len(frag.Days) > 0 {
				fragment = &frag
			}
		}
	}

	curated := curateMessages(in.Messages)
	ws := buildWorkingState(curated.Messages, "")
	inputItems, err := buildInputItems(curated.Messages)
	if err != nil {
		return turnResult{}, err
	}
	// Structured working state is authoritative for cancel/constraints; prose summary is gloss only.
	if wsJSON, err := json.Marshal(ws); err == nil {
		note := "Working state (JSON; honor cancel_this_turn and constraints; do not mutate when cancel_this_turn is true):\n" + string(wsJSON)
		inputItems = append([]responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfMessage(note, responses.EasyInputMessageRoleDeveloper),
		}, inputItems...)
	}
	inputItems = append([]responses.ResponseInputItemUnionParam{
		responses.ResponseInputItemParamOfMessage(
			"Turn fence: the latest user message is the only task this turn. "+
				"Prior assistant messages are historical — never reply as if you just performed those actions again. "+
				"If the user asks to remove/undo something, call the remove/undo tools; do not narrate a prior add as the current result.",
			responses.EasyInputMessageRoleDeveloper,
		),
	}, inputItems...)
	if curated.Summary != "" {
		note := "Earlier turns in this chat (lossy summary — prefer working state for constraints):\n" + curated.Summary
		inputItems = append([]responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfMessage(note, responses.EasyInputMessageRoleDeveloper),
		}, inputItems...)
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
		Instructions: openai.String(buildSystemPrompt(card, in.Day, prefs, learnings, fragment, in.FeedbackDown)),
		Tools:        tools,
		Store:        openai.Bool(true),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: inputItems,
		},
	}

	var result turnResult
	repairRounds := 0
	pendingRepair := false
	for iter := 0; iter < maxToolIterations; iter++ {
		resp, err := respond(ctx, params)
		if err != nil {
			return result, fmt.Errorf("openai: %w", err)
		}

		fnCalls := functionCalls(resp)
		if len(fnCalls) == 0 {
			if pendingRepair && repairRounds < maxRepairRounds {
				repairRounds++
				pendingRepair = false
				note := "HARNESS: invariants still need attention. Repair with tools or explain failure; do not claim a successful itinerary edit."
				params = responses.ResponseNewParams{
					Model:              a.model,
					PreviousResponseID: openai.String(resp.ID),
					Tools:              tools,
					Store:              openai.Bool(true),
					Input: responses.ResponseNewParamsInputUnion{
						OfInputItemList: []responses.ResponseInputItemUnionParam{
							responses.ResponseInputItemParamOfMessage(note, responses.EasyInputMessageRoleDeveloper),
						},
					},
				}
				continue
			}
			text := cleanAssistantText(resp.OutputText())
			if text != "" {
				if err := emit(Event{Type: "text", Text: text}); err != nil {
					return result, err
				}
			}
			return result, nil
		}

		outputs := make([]responses.ResponseInputItemUnionParam, 0, len(fnCalls)+1)
		needRepair := false
		var lastMutate string
		for _, fc := range fnCalls {
			if ws.CancelThisTurn && isMutateTool(fc.Name) {
				content := `{"error":"user cancelled this turn (e.g. NM / never mind / stop) — do not mutate; acknowledge and wait for a new clear ask"}`
				outputs = append(outputs, responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: fc.CallID,
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
							OfString: openai.String(content),
						},
					},
				})
				continue
			}
			out, patched, callErr := a.execTool(ctx, in, fc.Name, fc.Arguments)
			if patched {
				result.TripUpdated = true
				lastMutate = out
				if callErr == nil && invariantsNeedRepair(out) {
					needRepair = true
				}
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
		if needRepair && lastMutate != "" && repairRounds < maxRepairRounds {
			pendingRepair = true
			outputs = append(outputs, responses.ResponseInputItemParamOfMessage(
				repairDeveloperNote(lastMutate),
				responses.EasyInputMessageRoleDeveloper,
			))
			repairRounds++
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

func buildSystemPrompt(card TripCard, day int, prefs []Preference, learnings []Learning, fragment *TripFragment, down *FeedbackDown) string {
	var b strings.Builder
	b.WriteString(baseSystemPrompt())
	b.WriteString("Trip context (JSON):\n")
	raw, _ := json.Marshal(card)
	b.Write(raw)
	b.WriteByte('\n')
	if fragment != nil && len(fragment.Days) > 0 {
		b.WriteString("trip_fragment (JSON; day neighborhood orientation — not source of truth; call getTripYAML before mutate; default is day-scoped YAML, use scope=full only when needed):\n")
		fragJSON, _ := json.Marshal(fragment)
		b.Write(fragJSON)
		b.WriteByte('\n')
	}
	if len(prefs) == 0 {
		b.WriteString("Standing preferences: none saved yet.\n")
	} else {
		b.WriteString("Standing preferences (JSON; apply when choosing venues/logistics):\n")
		prefJSON, _ := json.Marshal(prefs)
		b.Write(prefJSON)
		b.WriteByte('\n')
	}
	if len(learnings) == 0 {
		b.WriteString("Agent learnings: none saved yet.\n")
	} else {
		b.WriteString("Agent learnings (JSON; how to operate this app — follow these):\n")
		learnJSON, _ := json.Marshal(learnings)
		b.Write(learnJSON)
		b.WriteByte('\n')
	}
	if down != nil && (strings.TrimSpace(down.UserText) != "" || strings.TrimSpace(down.AssistantText) != "") {
		b.WriteString("The user thumbs-downed a prior reply. Offer once this turn to save an agent learning (saveLearning) if they say what to do differently; only call saveLearning after they agree. Prior exchange:\n")
		b.WriteString("User: ")
		b.WriteString(truncateRunes(down.UserText, 300))
		b.WriteByte('\n')
		b.WriteString("Assistant: ")
		b.WriteString(truncateRunes(down.AssistantText, 300))
		b.WriteByte('\n')
	}
	if day > 0 {
		title := dayTitle(card, day)
		fmt.Fprintf(&b, "CURRENT VIEWER DAY: %d", day)
		if title != "" {
			fmt.Fprintf(&b, " — %s", title)
		}
		b.WriteString(".\n")
		b.WriteString("When the user says \"this day\", \"today\", \"the current day\", \"here\", or similar, they ALWAYS mean this CURRENT VIEWER DAY (the day shown in the viewer), not day 1 and not a day inferred from chat history. Call getTripYAML (day-scoped by default) before editing that day when you need its current stops/notes.\n")
	} else {
		b.WriteString("CURRENT VIEWER DAY: unknown (client did not send day). Ask which day if needed.\n")
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
