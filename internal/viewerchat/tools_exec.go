package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
		"getSchema":      handleGetSchema,
		"getTrip":        handleGetTrip,
		"getTripYAML":    handleGetTripYAML,
		"setDayPhoto":    handleSetDayPhoto,
		"listVersions":   handleListVersions,
		"getVersion":     handleGetVersion,
		"restoreVersion": handleRestoreVersion,
		"patchTrip":      handlePatchTrip,
	}
}

func (a *Agent) execTool(ctx context.Context, tripID, name, argsJSON string) (string, bool, error) {
	h, ok := toolHandlers()[name]
	if !ok {
		err := fmt.Errorf("unknown tool %q", name)
		logToolCall(tripID, name, argsJSON, false, err)
		return "", false, err
	}
	res, err := h(ctx, a, tripID, argsJSON)
	if err != nil {
		logToolCall(tripID, name, argsJSON, false, err)
		return "", false, err
	}
	logToolCall(tripID, name, argsJSON, res.Mutated, nil)
	return res.Content, res.Mutated, nil
}

func logToolCall(tripID, name, argsJSON string, mutated bool, err error) {
	args := compactJSONForLog(argsJSON, 400)
	if err != nil {
		log.Printf("viewerchat tool trip=%s name=%s mutated=false err=%v args=%s", tripID, name, err, args)
		return
	}
	log.Printf("viewerchat tool trip=%s name=%s mutated=%v args=%s", tripID, name, mutated, args)
}

func compactJSONForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "{}"
	}
	var raw any
	if json.Unmarshal([]byte(s), &raw) == nil {
		if b, err := json.Marshal(raw); err == nil {
			s = string(b)
		}
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func handleGetTrip(ctx context.Context, a *Agent, tripID, _ string) (toolResult, error) {
	card, err := a.ops.Summary(ctx, tripID)
	if err != nil {
		return toolResult{}, err
	}
	out := map[string]any{
		"id":             tripID,
		"schema_version": card.SchemaVersion,
		"trip":           card.Title,
		"description":    card.Description,
		"start":          card.Start,
		"days":           card.Days,
	}
	b, err := json.Marshal(out)
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

func handleSetDayPhoto(ctx context.Context, a *Agent, tripID, argsJSON string) (toolResult, error) {
	var args struct {
		Day          int    `json:"day"`
		Query        string `json:"query"`
		Photo        string `json:"photo"`
		PhotoCaption string `json:"photo_caption"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid setDayPhoto args: %w", err)
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
		return toolResult{}, fmt.Errorf("setDayPhoto did not persist: day %d photo is %q", args.Day, got)
	}
	if prev != "" && photoIdentity(prev) == photoIdentity(final) {
		return toolResult{}, fmt.Errorf("photo unchanged (still %s); try setDayPhoto with a different query", sourceTitle)
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

func handleListVersions(ctx context.Context, a *Agent, tripID, _ string) (toolResult, error) {
	vers, err := a.ops.ListVersions(ctx, tripID)
	if err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(map[string]any{
		"id":       tripID,
		"versions": vers,
		"count":    len(vers),
	})
	return toolResult{Content: string(b)}, err
}

func handleGetVersion(ctx context.Context, a *Agent, tripID, argsJSON string) (toolResult, error) {
	var args struct {
		VersionID string `json:"version_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid getVersion args: %w", err)
	}
	if strings.TrimSpace(args.VersionID) == "" {
		return toolResult{}, fmt.Errorf("version_id is required")
	}
	body, err := a.ops.GetYAMLVersion(ctx, tripID, args.VersionID)
	if err != nil {
		return toolResult{}, err
	}
	if len(body) > maxYAMLToolBytes {
		return toolResult{Content: string(body[:maxYAMLToolBytes]) + "\n…[truncated]"}, nil
	}
	return toolResult{Content: string(body)}, nil
}

func handleRestoreVersion(ctx context.Context, a *Agent, tripID, argsJSON string) (toolResult, error) {
	var args struct {
		VersionID string `json:"version_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid restoreVersion args: %w", err)
	}
	if strings.TrimSpace(args.VersionID) == "" {
		return toolResult{}, fmt.Errorf("version_id is required")
	}
	res, err := a.ops.RestoreVersion(ctx, tripID, args.VersionID)
	if err != nil {
		return toolResult{}, err
	}
	out := map[string]any{
		"ok":            true,
		"id":            res.ID,
		"version_id":    res.VersionID,
		"bundle_ok":     res.BundleOK,
		"restored_from": strings.TrimSpace(args.VersionID),
	}
	b, err := json.Marshal(out)
	return toolResult{Content: string(b), Mutated: true}, err
}

func handlePatchTrip(ctx context.Context, a *Agent, tripID, argsJSON string) (toolResult, error) {
	var wrap struct {
		Patch json.RawMessage `json:"patch"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &wrap); err != nil {
		return toolResult{}, fmt.Errorf("invalid patchTrip args: %w", err)
	}
	patch := wrap.Patch
	if len(patch) == 0 {
		patch = json.RawMessage(argsJSON)
	}
	rewritten, err := rewritePhotoURLsInPatch(ctx, patch)
	if err != nil {
		return toolResult{}, err
	}
	res, err := a.ops.Patch(ctx, tripID, rewritten)
	if err != nil {
		return toolResult{}, err
	}
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
