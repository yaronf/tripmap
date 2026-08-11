package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

type toolResult struct {
	Content string
	Mutated bool
}

type toolHandler func(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error)

func toolHandlers() map[string]toolHandler {
	return map[string]toolHandler{
		"getSchema":        handleGetSchema,
		"getTrip":          handleGetTrip,
		"getTripYAML":      handleGetTripYAML,
		"setDayPhoto":      handleSetDayPhoto,
		"listVersions":     handleListVersions,
		"getVersion":       handleGetVersion,
		"restoreVersion":   handleRestoreVersion,
		"patchTrip":        handlePatchTrip,
		"replaceDayRoutes": handleReplaceDayRoutes,
		"listPreferences":  handleListPreferences,
		"savePreference":   handleSavePreference,
		"forgetPreference": handleForgetPreference,
		"listLearnings":    handleListLearnings,
		"saveLearning":     handleSaveLearning,
		"forgetLearning":   handleForgetLearning,
	}
}

func (a *Agent) execTool(ctx context.Context, in TurnInput, name, argsJSON string) (string, bool, error) {
	h, ok := toolHandlers()[name]
	if !ok {
		err := fmt.Errorf("unknown tool %q", name)
		logToolCall(in.TripID, name, argsJSON, false, err)
		return "", false, err
	}
	res, err := h(ctx, a, in, argsJSON)
	if err != nil {
		logToolCall(in.TripID, name, argsJSON, false, err)
		return "", false, err
	}
	logToolCall(in.TripID, name, argsJSON, res.Mutated, nil)
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

func requireUserSub(in TurnInput) error {
	if strings.TrimSpace(in.UserSub) == "" {
		return fmt.Errorf("signed-in user required for preferences and learnings")
	}
	return nil
}

func handleGetTrip(ctx context.Context, a *Agent, in TurnInput, _ string) (toolResult, error) {
	card, err := a.ops.Summary(ctx, in.TripID)
	if err != nil {
		return toolResult{}, err
	}
	out := map[string]any{
		"id":             in.TripID,
		"schema_version": card.SchemaVersion,
		"trip":           card.Title,
		"description":    card.Description,
		"start":          card.Start,
		"days":           card.Days,
	}
	b, err := json.Marshal(out)
	return toolResult{Content: string(b)}, err
}

func handleGetSchema(ctx context.Context, a *Agent, _ TurnInput, _ string) (toolResult, error) {
	raw, err := a.ops.SchemaJSON(ctx)
	if err != nil {
		return toolResult{}, err
	}
	return toolResult{Content: string(raw)}, nil
}

func handleGetTripYAML(ctx context.Context, a *Agent, in TurnInput, _ string) (toolResult, error) {
	body, err := a.ops.GetYAML(ctx, in.TripID)
	if err != nil {
		return toolResult{}, err
	}
	if len(body) > maxYAMLToolBytes {
		return toolResult{Content: string(body[:maxYAMLToolBytes]) + "\n…[truncated]"}, nil
	}
	return toolResult{Content: string(body)}, nil
}

func handleSetDayPhoto(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
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
	if cur, err := a.ops.GetDay(ctx, in.TripID, args.Day); err == nil {
		prev = strings.TrimSpace(cur.Photo)
	}
	var exclude []string
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
	res, err := a.ops.Patch(ctx, in.TripID, patchJSON)
	if err != nil {
		return toolResult{}, err
	}
	got := ""
	if day, err := a.ops.GetDay(ctx, in.TripID, args.Day); err == nil {
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

func handleListVersions(ctx context.Context, a *Agent, in TurnInput, _ string) (toolResult, error) {
	vers, err := a.ops.ListVersions(ctx, in.TripID)
	if err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(map[string]any{
		"id":       in.TripID,
		"versions": vers,
		"count":    len(vers),
	})
	return toolResult{Content: string(b)}, err
}

func handleGetVersion(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
	var args struct {
		VersionID string `json:"version_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid getVersion args: %w", err)
	}
	if strings.TrimSpace(args.VersionID) == "" {
		return toolResult{}, fmt.Errorf("version_id is required")
	}
	body, err := a.ops.GetYAMLVersion(ctx, in.TripID, args.VersionID)
	if err != nil {
		return toolResult{}, err
	}
	if len(body) > maxYAMLToolBytes {
		return toolResult{Content: string(body[:maxYAMLToolBytes]) + "\n…[truncated]"}, nil
	}
	return toolResult{Content: string(body)}, nil
}

func handleRestoreVersion(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
	var args struct {
		VersionID string `json:"version_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid restoreVersion args: %w", err)
	}
	if strings.TrimSpace(args.VersionID) == "" {
		return toolResult{}, fmt.Errorf("version_id is required")
	}
	res, err := a.ops.RestoreVersion(ctx, in.TripID, args.VersionID)
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

func handlePatchTrip(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
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
	res, err := a.ops.Patch(ctx, in.TripID, rewritten)
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

func handleReplaceDayRoutes(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
	p, err := itinerary.ParseReplaceDayRoutes([]byte(argsJSON))
	if err != nil {
		return toolResult{}, err
	}
	patch, err := json.Marshal(p)
	if err != nil {
		return toolResult{}, err
	}
	res, err := a.ops.Patch(ctx, in.TripID, patch)
	if err != nil {
		return toolResult{}, err
	}
	out := map[string]any{
		"ok":         true,
		"id":         res.ID,
		"version_id": res.VersionID,
		"bundle_ok":  res.BundleOK,
		"op":         "replaceDayRoutes",
	}
	if body, err := a.ops.GetYAML(ctx, in.TripID); err == nil {
		if warns := ContinuityWarnings(body, dayNumsFromReplaceArgs(argsJSON)); len(warns) > 0 {
			out["warnings"] = warns
		}
		// Echo applied routes for the days touched so the model can self-check types.
		if frag, err := BuildTripFragment(body, firstDayOr(in.Day, dayNumsFromReplaceArgs(argsJSON))); err == nil && len(frag.Days) > 0 {
			out["trip_fragment"] = frag
		}
	}
	b, err := json.Marshal(out)
	return toolResult{Content: string(b), Mutated: true}, err
}

func firstDayOr(viewerDay int, nums []int) int {
	if len(nums) > 0 {
		min := nums[0]
		for _, n := range nums[1:] {
			if n < min {
				min = n
			}
		}
		return min
	}
	if viewerDay > 0 {
		return viewerDay
	}
	return 1
}

func handleListPreferences(ctx context.Context, a *Agent, in TurnInput, _ string) (toolResult, error) {
	if err := requireUserSub(in); err != nil {
		return toolResult{}, err
	}
	prefs, err := a.ops.ListPreferences(ctx, in.UserSub)
	if err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(map[string]any{"preferences": prefs, "count": len(prefs)})
	return toolResult{Content: string(b)}, err
}

func handleSavePreference(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
	if err := requireUserSub(in); err != nil {
		return toolResult{}, err
	}
	var args struct {
		PreferenceID string   `json:"preference_id"`
		Text         string   `json:"text"`
		Tags         []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid savePreference args: %w", err)
	}
	pref, err := a.ops.SavePreference(ctx, in.UserSub, args.PreferenceID, args.Text, args.Tags)
	if err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(map[string]any{"ok": true, "preference": pref})
	return toolResult{Content: string(b)}, err
}

func handleForgetPreference(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
	if err := requireUserSub(in); err != nil {
		return toolResult{}, err
	}
	var args struct {
		PreferenceID string `json:"preference_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid forgetPreference args: %w", err)
	}
	if err := a.ops.ForgetPreference(ctx, in.UserSub, args.PreferenceID); err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(map[string]any{"ok": true, "forgotten": args.PreferenceID})
	return toolResult{Content: string(b)}, err
}

func handleListLearnings(ctx context.Context, a *Agent, in TurnInput, _ string) (toolResult, error) {
	if err := requireUserSub(in); err != nil {
		return toolResult{}, err
	}
	items, err := a.ops.ListLearnings(ctx, in.UserSub)
	if err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(map[string]any{"learnings": items, "count": len(items)})
	return toolResult{Content: string(b)}, err
}

func handleSaveLearning(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
	if err := requireUserSub(in); err != nil {
		return toolResult{}, err
	}
	var args struct {
		LearningID string   `json:"learning_id"`
		Text       string   `json:"text"`
		Tags       []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid saveLearning args: %w", err)
	}
	item, err := a.ops.SaveLearning(ctx, in.UserSub, args.LearningID, args.Text, args.Tags)
	if err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(map[string]any{"ok": true, "learning": item})
	return toolResult{Content: string(b)}, err
}

func handleForgetLearning(ctx context.Context, a *Agent, in TurnInput, argsJSON string) (toolResult, error) {
	if err := requireUserSub(in); err != nil {
		return toolResult{}, err
	}
	var args struct {
		LearningID string `json:"learning_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid forgetLearning args: %w", err)
	}
	if err := a.ops.ForgetLearning(ctx, in.UserSub, args.LearningID); err != nil {
		return toolResult{}, err
	}
	b, err := json.Marshal(map[string]any{"ok": true, "forgotten": args.LearningID})
	return toolResult{Content: string(b)}, err
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
