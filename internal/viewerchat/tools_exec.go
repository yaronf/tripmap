package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type toolResult struct {
	Content string
	Mutated bool
}

type toolHandler func(ctx context.Context, a *Agent, tripID, argsJSON string) (toolResult, error)

func toolHandlers() map[string]toolHandler {
	return map[string]toolHandler{
		"get_trip_summary": handleGetTripSummary,
		"get_schema":       handleGetSchema,
		"get_trip_yaml":    handleGetTripYAML,
		"get_day":          handleGetDay,
		"set_day_photo":    handleSetDayPhoto,
		"patch_trip":       handlePatchTrip,
	}
}

func (a *Agent) execTool(ctx context.Context, tripID, name, argsJSON string) (string, bool, error) {
	h, ok := toolHandlers()[name]
	if !ok {
		return "", false, fmt.Errorf("unknown tool %q", name)
	}
	res, err := h(ctx, a, tripID, argsJSON)
	if err != nil {
		return "", false, err
	}
	return res.Content, res.Mutated, nil
}

func handleGetTripSummary(ctx context.Context, a *Agent, tripID, _ string) (toolResult, error) {
	card, err := a.ops.Summary(ctx, tripID)
	if err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(card)
	return toolResult{Content: string(b)}, err
}

func handleGetSchema(ctx context.Context, a *Agent, _, _ string) (toolResult, error) {
	raw, err := a.ops.SchemaJSON(ctx)
	if err != nil {
		return toolResult{}, err
	}
	return toolResult{Content: string(raw)}, nil
}

func handleGetTripYAML(ctx context.Context, a *Agent, tripID, _ string) (toolResult, error) {
	body, err := a.ops.GetYAML(ctx, tripID)
	if err != nil {
		return toolResult{}, err
	}
	if len(body) > maxYAMLToolBytes {
		return toolResult{Content: string(body[:maxYAMLToolBytes]) + "\n…[truncated]"}, nil
	}
	return toolResult{Content: string(body)}, nil
}

func handleGetDay(ctx context.Context, a *Agent, tripID, argsJSON string) (toolResult, error) {
	var args struct {
		Day int `json:"day"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || args.Day < 1 {
		return toolResult{}, fmt.Errorf("get_day requires {\"day\": <1-based day number>}")
	}
	day, err := a.ops.GetDay(ctx, tripID, args.Day)
	if err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(day)
	return toolResult{Content: string(b)}, err
}

func handleSetDayPhoto(ctx context.Context, a *Agent, tripID, argsJSON string) (toolResult, error) {
	var args struct {
		Day          int    `json:"day"`
		Query        string `json:"query"`
		Photo        string `json:"photo"`
		PhotoCaption string `json:"photo_caption"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid set_day_photo args: %w", err)
	}
	if args.Day < 1 {
		return toolResult{}, fmt.Errorf("day must be >= 1")
	}
	prev := ""
	if cur, err := a.ops.GetDay(ctx, tripID, args.Day); err == nil {
		prev = strings.TrimSpace(cur.Photo)
	}
	var exclude []string
	// Skip the current day photo when searching by query so "different photo"
	// cannot re-select the same Commons file.
	if prev != "" && strings.TrimSpace(args.Photo) == "" {
		exclude = append(exclude, prev)
	}
	final, sourceTitle, err := resolvePhotoForTrip(ctx, args.Photo, args.Query, exclude)
	if err != nil {
		return toolResult{}, err
	}
	caption := strings.TrimSpace(args.PhotoCaption)
	ud := map[string]any{
		"day":   args.Day,
		"photo": final,
	}
	if caption != "" {
		ud["photo_caption"] = caption
	}
	patchJSON, err := json.Marshal(map[string]any{"update_day": ud})
	if err != nil {
		return toolResult{}, err
	}
	res, err := a.ops.Patch(ctx, tripID, patchJSON)
	if err != nil {
		return toolResult{}, err
	}
	got := ""
	if day, err := a.ops.GetDay(ctx, tripID, args.Day); err == nil {
		got = strings.TrimSpace(day.Photo)
	}
	if photoIdentity(got) != photoIdentity(final) {
		return toolResult{}, fmt.Errorf("set_day_photo did not persist: day %d photo is %q", args.Day, got)
	}
	if prev != "" && photoIdentity(prev) == photoIdentity(final) {
		return toolResult{}, fmt.Errorf("photo unchanged (still %s); try set_day_photo with a different query", sourceTitle)
	}
	out := map[string]any{
		"ok":         true,
		"id":         res.ID,
		"version_id": res.VersionID,
		"bundle_ok":  res.BundleOK,
		"day":        args.Day,
		"photo":      final,
		"previous":   prev,
	}
	if sourceTitle != "" {
		out["commons_title"] = sourceTitle
	}
	if caption != "" {
		out["photo_caption"] = caption
	}
	b, err := json.Marshal(out)
	return toolResult{Content: string(b), Mutated: true}, err
}

func handlePatchTrip(ctx context.Context, a *Agent, tripID, argsJSON string) (toolResult, error) {
	var wrap struct {
		Patch json.RawMessage `json:"patch"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &wrap); err != nil {
		return toolResult{}, fmt.Errorf("invalid patch_trip args: %w", err)
	}
	if len(wrap.Patch) == 0 {
		wrap.Patch = json.RawMessage(argsJSON)
	}
	rewritten, err := rewritePhotoURLsInPatch(ctx, wrap.Patch)
	if err != nil {
		return toolResult{}, err
	}
	res, err := a.ops.Patch(ctx, tripID, rewritten)
	if err != nil {
		return toolResult{}, err
	}
	// Domain invariants (e.g. remove_stop must remove) live in itinerary.ApplyPatch.
	out := map[string]any{
		"ok":         true,
		"id":         res.ID,
		"version_id": res.VersionID,
		"bundle_ok":  res.BundleOK,
		"ops":        patchOpNames(rewritten),
	}
	b, err := json.Marshal(out)
	return toolResult{Content: string(b), Mutated: true}, err
}

func patchOpNames(patchJSON json.RawMessage) []string {
	var raw map[string]json.RawMessage
	if json.Unmarshal(patchJSON, &raw) != nil {
		return nil
	}
	keys := make([]string, 0, len(raw))
	for k, v := range raw {
		if len(v) == 0 || string(v) == "null" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
