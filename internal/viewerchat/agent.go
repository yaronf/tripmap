package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/yaronf/tripmap/internal/itinerary"
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

func (a *Agent) execTool(ctx context.Context, tripID, name, argsJSON string) (string, bool, error) {
	switch name {
	case "get_trip_summary":
		card, err := a.ops.Summary(ctx, tripID)
		if err != nil {
			return "", false, err
		}
		b, err := json.Marshal(card)
		return string(b), false, err
	case "get_schema":
		raw, err := a.ops.SchemaJSON(ctx)
		if err != nil {
			return "", false, err
		}
		return string(raw), false, nil
	case "get_trip_yaml":
		body, err := a.ops.GetYAML(ctx, tripID)
		if err != nil {
			return "", false, err
		}
		if len(body) > maxYAMLToolBytes {
			return string(body[:maxYAMLToolBytes]) + "\n…[truncated]", false, nil
		}
		return string(body), false, nil
	case "get_day":
		var args struct {
			Day int `json:"day"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || args.Day < 1 {
			return "", false, fmt.Errorf("get_day requires {\"day\": <1-based day number>}")
		}
		body, err := a.ops.GetYAML(ctx, tripID)
		if err != nil {
			return "", false, err
		}
		trip, err := itinerary.ParseYAML(body)
		if err != nil {
			return "", false, err
		}
		var day *itinerary.Day
		for i := range trip.Days {
			if trip.Days[i].Day == args.Day {
				day = &trip.Days[i]
				break
			}
		}
		if day == nil {
			return "", false, fmt.Errorf("day %d not found", args.Day)
		}
		type stopOut struct {
			Place string `json:"place"`
			Title string `json:"title,omitempty"`
			Type  string `json:"type,omitempty"`
			Notes string `json:"notes,omitempty"`
			List  string `json:"list"`
		}
		outStops := make([]stopOut, 0, len(day.Stops)+len(day.Route))
		for _, s := range day.Route {
			title := s.Place
			if p, ok := trip.Places[s.Place]; ok && p.Title != "" {
				title = p.Title
			}
			typ := s.Type
			if typ == "" {
				typ = trip.Places[s.Place].Type
			}
			outStops = append(outStops, stopOut{Place: s.Place, Title: title, Type: typ, Notes: s.Notes, List: "route"})
		}
		for _, s := range day.Stops {
			title := s.Place
			if p, ok := trip.Places[s.Place]; ok && p.Title != "" {
				title = p.Title
			}
			typ := s.Type
			if typ == "" {
				typ = trip.Places[s.Place].Type
			}
			outStops = append(outStops, stopOut{Place: s.Place, Title: title, Type: typ, Notes: s.Notes, List: "stops"})
		}
		b, err := json.Marshal(map[string]any{
			"day":   day.Day,
			"title": day.Title,
			"notes": day.Notes,
			"stops": outStops,
		})
		return string(b), false, err
	case "set_day_photo":
		var args struct {
			Day          int    `json:"day"`
			Query        string `json:"query"`
			Photo        string `json:"photo"`
			PhotoCaption string `json:"photo_caption"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", false, fmt.Errorf("invalid set_day_photo args: %w", err)
		}
		if args.Day < 1 {
			return "", false, fmt.Errorf("day must be >= 1")
		}
		prev := a.currentDayPhoto(ctx, tripID, args.Day)
		var exclude []string
		// Always skip the current day photo when searching by query so
		// "different/another photo" cannot re-select the same Commons file.
		if prev != "" && strings.TrimSpace(args.Photo) == "" {
			exclude = append(exclude, prev)
		}
		final, sourceTitle, err := resolvePhotoForTrip(ctx, args.Photo, args.Query, exclude)
		if err != nil {
			return "", false, err
		}
		caption := strings.TrimSpace(args.PhotoCaption)
		ud := map[string]any{
			"day":   args.Day,
			"photo": final,
		}
		if caption != "" {
			ud["photo_caption"] = caption
		}
		patchObj := map[string]any{"update_day": ud}
		patchJSON, err := json.Marshal(patchObj)
		if err != nil {
			return "", false, err
		}
		res, err := a.ops.Patch(ctx, tripID, patchJSON)
		if err != nil {
			return "", false, err
		}
		verify := a.verifyDayPhoto(ctx, tripID, args.Day, final)
		if ok, _ := verify["photo_set"].(bool); !ok {
			return "", false, fmt.Errorf("set_day_photo did not persist: day %d photo is %q", args.Day, verify["photo"])
		}
		changed := prev == "" || photoIdentity(prev) != photoIdentity(final)
		out := map[string]any{
			"ok":         true,
			"id":         res.ID,
			"version_id": res.VersionID,
			"bundle_ok":  res.BundleOK,
			"day":        args.Day,
			"photo":      final,
			"changed":    changed,
			"previous":   prev,
			"verify":     verify,
		}
		if sourceTitle != "" {
			out["commons_title"] = sourceTitle
		}
		if caption != "" {
			out["photo_caption"] = caption
		}
		if !changed {
			return "", false, fmt.Errorf("photo unchanged (still %s); try set_day_photo with a different query", sourceTitle)
		}
		b, err := json.Marshal(out)
		return string(b), true, err
	case "patch_trip":
		var wrap struct {
			Patch json.RawMessage `json:"patch"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &wrap); err != nil {
			return "", false, fmt.Errorf("invalid patch_trip args: %w", err)
		}
		if len(wrap.Patch) == 0 {
			wrap.Patch = json.RawMessage(argsJSON)
		}
		rewritten, err := rewritePhotoURLsInPatch(ctx, wrap.Patch)
		if err != nil {
			return "", false, err
		}
		wrap.Patch = rewritten
		res, err := a.ops.Patch(ctx, tripID, wrap.Patch)
		if err != nil {
			return "", false, err
		}
		out := map[string]any{
			"ok":         true,
			"id":         res.ID,
			"version_id": res.VersionID,
			"bundle_ok":  res.BundleOK,
			"ops":        patchOpNames(wrap.Patch),
		}
		if verify := a.patchVerify(ctx, tripID, wrap.Patch); verify != nil {
			out["verify"] = verify
			// Hard fail if a remove left the place on the day (should not happen).
			if still, ok := verify["still_present"].(bool); ok && still {
				return "", false, fmt.Errorf("remove_stop did not remove the place; it is still on the day")
			}
			if left, ok := verify["still_present_places"].([]string); ok && len(left) > 0 {
				return "", false, fmt.Errorf("remove_stop incomplete; still present: %s", strings.Join(left, ", "))
			}
			if set, ok := verify["photo_set"].(bool); ok && !set {
				return "", false, fmt.Errorf("update_day photo did not persist on day %v", verify["day"])
			}
		}
		b, err := json.Marshal(out)
		return string(b), true, err
	default:
		return "", false, fmt.Errorf("unknown tool %q", name)
	}
}

func patchOpNames(patchJSON json.RawMessage) []string {
	var p struct {
		SwapDays   json.RawMessage `json:"swap_days"`
		Days       json.RawMessage `json:"days"`
		UpdateDay  json.RawMessage `json:"update_day"`
		Places     json.RawMessage `json:"places"`
		UpsertStop json.RawMessage `json:"upsert_stop"`
		RemoveStop json.RawMessage `json:"remove_stop"`
		InsertDay  json.RawMessage `json:"insert_day"`
		DeleteDay  json.RawMessage `json:"delete_day"`
	}
	if json.Unmarshal(patchJSON, &p) != nil {
		return nil
	}
	var ops []string
	if len(p.SwapDays) > 0 && string(p.SwapDays) != "null" {
		ops = append(ops, "swap_days")
	}
	if len(p.Days) > 0 && string(p.Days) != "null" {
		ops = append(ops, "days")
	}
	if len(p.UpdateDay) > 0 && string(p.UpdateDay) != "null" {
		ops = append(ops, "update_day")
	}
	if len(p.Places) > 0 && string(p.Places) != "null" {
		ops = append(ops, "places")
	}
	if len(p.UpsertStop) > 0 && string(p.UpsertStop) != "null" {
		ops = append(ops, "upsert_stop")
	}
	if len(p.RemoveStop) > 0 && string(p.RemoveStop) != "null" {
		ops = append(ops, "remove_stop")
	}
	if len(p.InsertDay) > 0 && string(p.InsertDay) != "null" {
		ops = append(ops, "insert_day")
	}
	if len(p.DeleteDay) > 0 && string(p.DeleteDay) != "null" {
		ops = append(ops, "delete_day")
	}
	return ops
}

func (a *Agent) currentDayPhoto(ctx context.Context, tripID string, day int) string {
	body, err := a.ops.GetYAML(ctx, tripID)
	if err != nil {
		return ""
	}
	trip, err := itinerary.ParseYAML(body)
	if err != nil {
		return ""
	}
	for _, d := range trip.Days {
		if d.Day == day {
			return strings.TrimSpace(d.Photo)
		}
	}
	return ""
}

func (a *Agent) verifyDayPhoto(ctx context.Context, tripID string, day int, want string) map[string]any {
	out := map[string]any{"day": day, "want": want, "photo_set": false}
	body, err := a.ops.GetYAML(ctx, tripID)
	if err != nil {
		out["warning"] = "could not re-read trip after patch"
		return out
	}
	trip, err := itinerary.ParseYAML(body)
	if err != nil {
		out["warning"] = "could not parse trip after patch"
		return out
	}
	for _, d := range trip.Days {
		if d.Day != day {
			continue
		}
		out["photo"] = d.Photo
		out["photo_caption"] = d.PhotoCaption
		out["photo_set"] = strings.TrimSpace(d.Photo) != "" && (want == "" || d.Photo == want)
		return out
	}
	out["warning"] = "day not found"
	return out
}

func (a *Agent) patchVerify(ctx context.Context, tripID string, patchJSON json.RawMessage) map[string]any {
	var p struct {
		UpdateDay *struct {
			Day   int     `json:"day"`
			Photo *string `json:"photo"`
		} `json:"update_day"`
		UpsertStop *struct {
			Day   int    `json:"day"`
			Place string `json:"place"`
			List  string `json:"list"`
		} `json:"upsert_stop"`
		RemoveStop *struct {
			Day    int      `json:"day"`
			Place  string   `json:"place"`
			Places []string `json:"places"`
			List   string   `json:"list"`
		} `json:"remove_stop"`
		Places map[string]any `json:"places"`
	}
	if json.Unmarshal(patchJSON, &p) != nil {
		return nil
	}

	body, err := a.ops.GetYAML(ctx, tripID)
	if err != nil {
		return map[string]any{"warning": "could not re-read trip after patch"}
	}
	trip, err := itinerary.ParseYAML(body)
	if err != nil {
		return map[string]any{"warning": "could not parse trip after patch"}
	}

	if p.UpdateDay != nil && p.UpdateDay.Day > 0 && p.UpdateDay.Photo != nil {
		want := strings.TrimSpace(*p.UpdateDay.Photo)
		out := map[string]any{"day": p.UpdateDay.Day, "want": want, "photo_set": false}
		for _, d := range trip.Days {
			if d.Day != p.UpdateDay.Day {
				continue
			}
			out["photo"] = d.Photo
			out["photo_caption"] = d.PhotoCaption
			out["photo_set"] = strings.TrimSpace(d.Photo) != "" && d.Photo == want
			return out
		}
		out["warning"] = "day not found"
		return out
	}

	if p.RemoveStop != nil && p.RemoveStop.Day > 0 {
		wanted := make([]string, 0, 1+len(p.RemoveStop.Places))
		if strings.TrimSpace(p.RemoveStop.Place) != "" {
			wanted = append(wanted, p.RemoveStop.Place)
		}
		wanted = append(wanted, p.RemoveStop.Places...)
		out := map[string]any{
			"day":     p.RemoveStop.Day,
			"list":    p.RemoveStop.List,
			"removed": wanted,
		}
		for _, d := range trip.Days {
			if d.Day != p.RemoveStop.Day {
				continue
			}
			onDay := map[string]string{} // id -> title
			collect := func(list []itinerary.Stop) {
				for _, s := range list {
					title := s.Place
					if pl, ok := trip.Places[s.Place]; ok && pl.Title != "" {
						title = pl.Title
					}
					onDay[s.Place] = title
				}
			}
			switch p.RemoveStop.List {
			case "route":
				collect(d.Route)
			case "stops":
				collect(d.Stops)
			default:
				collect(d.Route)
				collect(d.Stops)
			}
			places := make([]string, 0, len(onDay))
			for id := range onDay {
				places = append(places, id)
			}
			out["stop_places"] = places
			var still []string
			for _, w := range wanted {
				wl := strings.ToLower(strings.TrimSpace(w))
				if _, ok := onDay[w]; ok {
					still = append(still, w)
					continue
				}
				for id, title := range onDay {
					if strings.ToLower(title) == wl {
						still = append(still, id)
						break
					}
				}
			}
			out["still_present"] = len(still) > 0
			out["still_present_places"] = still
			break
		}
		return out
	}

	if p.UpsertStop != nil && p.UpsertStop.Day > 0 {
		out := map[string]any{
			"day":            p.UpsertStop.Day,
			"upserted_place": p.UpsertStop.Place,
			"list":           p.UpsertStop.List,
		}
		if p.Places != nil {
			_, out["place_created_in_same_patch"] = p.Places[p.UpsertStop.Place]
		}
		for _, d := range trip.Days {
			if d.Day != p.UpsertStop.Day {
				continue
			}
			list := d.Stops
			if p.UpsertStop.List == "route" {
				list = d.Route
			}
			places := make([]string, 0, len(list))
			present := false
			for _, s := range list {
				places = append(places, s.Place)
				if s.Place == p.UpsertStop.Place {
					present = true
				}
			}
			out["stop_places"] = places
			out["stop_present"] = present
			break
		}
		return out
	}

	if len(p.Places) > 0 {
		ids := make([]string, 0, len(p.Places))
		for id := range p.Places {
			ids = append(ids, id)
		}
		return map[string]any{"place_ids": ids}
	}
	return nil
}
