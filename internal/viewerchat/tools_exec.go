package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

type toolResult struct {
	Content string
	Mutated bool
}

type toolHandler func(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error)

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
		"changeOvernight":  handleChangeOvernight,
		"listPreferences":  handleListPreferences,
		"savePreference":   handleSavePreference,
		"forgetPreference": handleForgetPreference,
		"listLearnings":    handleListLearnings,
		"saveLearning":     handleSaveLearning,
		"forgetLearning":   handleForgetLearning,
	}
}

func (a *Agent) execTool(ctx context.Context, in TurnInput, name, argsJSON string, tc *turnToolContext) (string, bool, error) {
	h, ok := toolHandlers()[name]
	if !ok {
		err := fmt.Errorf("unknown tool %q", name)
		logToolCall(in.TripID, name, argsJSON, false, err)
		return "", false, err
	}
	res, err := h(ctx, a, in, argsJSON, tc)
	if err != nil {
		logToolCall(in.TripID, name, argsJSON, false, err)
		return "", false, err
	}
	if name == "getTripYAML" && err == nil {
		if tc != nil {
			tc.SawGetTripYAML = true
		}
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

func handleGetTrip(ctx context.Context, a *Agent, in TurnInput, _ string, tc *turnToolContext) (toolResult, error) {
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

func handleGetSchema(ctx context.Context, a *Agent, _ TurnInput, _ string, tc *turnToolContext) (toolResult, error) {
	raw, err := a.ops.SchemaJSON(ctx)
	if err != nil {
		return toolResult{}, err
	}
	return toolResult{Content: string(raw)}, nil
}

func handleGetTripYAML(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
	var args struct {
		Day   int    `json:"day"`
		Scope string `json:"scope"`
	}
	if strings.TrimSpace(argsJSON) != "" && argsJSON != "{}" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return toolResult{}, fmt.Errorf("invalid getTripYAML args: %w", err)
		}
	}
	scope := strings.ToLower(strings.TrimSpace(args.Scope))
	day := args.Day
	if day < 1 {
		day = in.Day
	}
	// Chat default: day-neighborhood SoT. Fall back to full when there is no day.
	if scope == "" {
		if day >= 1 {
			scope = "day"
		} else {
			scope = "full"
		}
	}
	switch scope {
	case "day", "full":
	default:
		return toolResult{}, fmt.Errorf("scope must be \"day\" or \"full\"")
	}

	body, err := a.ops.GetYAML(ctx, in.TripID)
	if err != nil {
		return toolResult{}, err
	}
	if scope == "day" {
		if day < 1 {
			return toolResult{}, fmt.Errorf("day is required when scope=day")
		}
		scoped, err := BuildDayScopedYAML(body, day)
		if err != nil {
			return toolResult{}, err
		}
		return toolResult{Content: string(scoped)}, nil
	}
	if len(body) > maxYAMLToolBytes {
		return toolResult{Content: string(body[:maxYAMLToolBytes]) + "\n…[truncated]"}, nil
	}
	return toolResult{Content: string(body)}, nil
}

func handleSetDayPhoto(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
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

func handleListVersions(ctx context.Context, a *Agent, in TurnInput, _ string, tc *turnToolContext) (toolResult, error) {
	vers, err := a.ops.ListVersions(ctx, in.TripID)
	if err != nil {
		return toolResult{}, err
	}
	undoHint := ""
	for _, v := range vers {
		if !v.IsLatest {
			undoHint = v.VersionID
			break
		}
	}
	out := map[string]any{
		"id":       in.TripID,
		"versions": vers,
		"count":    len(vers),
		"undo_hint": "To undo the latest change, restoreVersion with the first is_latest=false entry (undo_version_id if set).",
	}
	if undoHint != "" {
		out["undo_version_id"] = undoHint
	}
	b, err := json.Marshal(out)
	return toolResult{Content: string(b)}, err
}

func handleGetVersion(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
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

func handleRestoreVersion(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
	var args struct {
		VersionID string `json:"version_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid restoreVersion args: %w", err)
	}
	if strings.TrimSpace(args.VersionID) == "" {
		return toolResult{}, fmt.Errorf("version_id is required — call listVersions first; to undo latest, use undo_version_id (first is_latest=false)")
	}
	vers, _ := a.ops.ListVersions(ctx, in.TripID)
	for _, v := range vers {
		if v.IsLatest && v.VersionID == strings.TrimSpace(args.VersionID) {
			return toolResult{}, fmt.Errorf(
				"refusing to restore is_latest version_id %q (that rewrites the same tip). "+
					"To undo the latest change, restore the first entry with is_latest=false from listVersions",
				args.VersionID,
			)
		}
	}
	res, err := a.ops.RestoreVersion(ctx, in.TripID, args.VersionID)
	if err != nil {
		return toolResult{}, err
	}
	body, _ := a.ops.GetYAML(ctx, in.TripID)
	content, err := enrichAfterMutate(body, in.Day, nil, mutateResult{
		OK: true, Op: "restoreVersion", ID: res.ID, VersionID: res.VersionID, BundleOK: res.BundleOK,
		Changed: map[string]any{"restored_from": strings.TrimSpace(args.VersionID)},
	})
	return toolResult{Content: content, Mutated: true}, err
}

func handlePatchTrip(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
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
	ask := latestUserAsk(in.Messages)
	if err := rejectRemoveIntentMisuse(patch, ask); err != nil {
		return toolResult{}, err
	}
	if err := rejectRemoveWithoutYAML(patch, tc); err != nil {
		return toolResult{}, err
	}
	if err := rejectStopsWhenNeedsMidRoute(patch, ask); err != nil {
		return toolResult{}, err
	}
	before, err := a.ops.GetYAML(ctx, in.TripID)
	if err != nil {
		return toolResult{}, err
	}
	if err := rejectChatStructuralPatch(patch, before, ask); err != nil {
		return toolResult{}, err
	}
	if err := rejectWeakNewPlaces(patch, before); err != nil {
		return toolResult{}, err
	}
	rewritten, err := rewritePhotoURLsInPatch(ctx, patch)
	if err != nil {
		return toolResult{}, err
	}
	res, err := a.ops.Patch(ctx, in.TripID, rewritten)
	if err != nil {
		return toolResult{}, err
	}
	after, _ := a.ops.GetYAML(ctx, in.TripID)
	var p itinerary.Patch
	_ = json.Unmarshal(rewritten, &p)
	content, err := enrichAfterMutate(after, in.Day, patchDayNums(p), mutateResult{
		OK: true, Op: "patchTrip", ID: res.ID, VersionID: res.VersionID, BundleOK: res.BundleOK,
		Changed: patchSummaryChanged(before, after, p),
		Extra:   map[string]any{"ops": patchOpNames(rewritten)},
	})
	return toolResult{Content: content, Mutated: true}, err
}

func handleReplaceDayRoutes(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
	_ = tc
	ask := latestUserAsk(in.Messages)
	if err := rejectReplaceUnlessRouteSurgery(ask); err != nil {
		return toolResult{}, err
	}
	dayNums := dayNumsFromReplaceArgs(argsJSON)
	if err := rejectReplaceOutsideAskScope(dayNums, in.Day, ask); err != nil {
		return toolResult{}, err
	}
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
	after, _ := a.ops.GetYAML(ctx, in.TripID)
	content, err := enrichAfterMutate(after, in.Day, dayNums, mutateResult{
		OK: true, Op: "replaceDayRoutes", ID: res.ID, VersionID: res.VersionID, BundleOK: res.BundleOK,
		Changed: map[string]any{"days": dayNums},
	})
	return toolResult{Content: content, Mutated: true}, err
}

func handleChangeOvernight(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
	var args struct {
		Day                   int            `json:"day"`
		NewEnd                string         `json:"new_end"`
		Title                 string         `json:"title"`
		AlsoUpdateNextStart   *bool          `json:"also_update_next_start"`
		Force                 bool           `json:"force"`
		Places                map[string]any `json:"places"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return toolResult{}, fmt.Errorf("invalid changeOvernight args: %w", err)
	}
	body, err := a.ops.GetYAML(ctx, in.TripID)
	if err != nil {
		return toolResult{}, err
	}
	trip, err := itinerary.ParseYAML(body)
	if err != nil {
		return toolResult{}, err
	}
	coa := itinerary.ChangeOvernightArgs{
		Day:    args.Day,
		NewEnd: args.NewEnd,
		Title:  args.Title,
		Force:  args.Force,
	}
	if args.AlsoUpdateNextStart != nil {
		coa.AlsoUpdateNextStart = *args.AlsoUpdateNextStart
		coa.AlsoUpdateNextStartSet = true
	}
	if args.Places != nil {
		if raw, ok := args.Places[strings.TrimSpace(args.NewEnd)]; ok {
			b, _ := json.Marshal(raw)
			var pl itinerary.Place
			if err := json.Unmarshal(b, &pl); err != nil {
				return toolResult{}, fmt.Errorf("changeOvernight: places.%s: %w", args.NewEnd, err)
			}
			coa.NewPlace = &pl
		} else {
			// Allow creating any places in the map keyed by new_end only for now.
			for id, raw := range args.Places {
				if id != strings.TrimSpace(args.NewEnd) {
					continue
				}
				b, _ := json.Marshal(raw)
				var pl itinerary.Place
				if err := json.Unmarshal(b, &pl); err != nil {
					return toolResult{}, fmt.Errorf("changeOvernight: places.%s: %w", id, err)
				}
				coa.NewPlace = &pl
			}
		}
	}
	domainRes, err := itinerary.ApplyChangeOvernight(&trip, coa)
	if err != nil {
		return toolResult{}, err
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		return toolResult{}, err
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		return toolResult{}, err
	}
	res, err := a.ops.CommitYAML(ctx, in.TripID, outYAML)
	if err != nil {
		return toolResult{}, err
	}
	changed := map[string]any{
		"day":     domainRes.Day,
		"old_end": domainRes.OldEnd,
		"new_end": domainRes.NewEnd,
	}
	if domainRes.DistanceKM > 0 {
		changed["distance_km"] = math.Round(domainRes.DistanceKM)
	}
	derived := map[string]any{}
	preserved := []string{}
	if domainRes.NextDay > 0 {
		derived["day_"+strconv.Itoa(domainRes.NextDay)+"_start"] = map[string]any{
			"old": domainRes.OldNextStart,
			"new": domainRes.NewNextStart,
		}
		if domainRes.PreservedNextMid {
			preserved = append(preserved, fmt.Sprintf("day_%d.route.mid_and_end", domainRes.NextDay))
		}
	}
	dayNums := []int{domainRes.Day}
	if domainRes.NextDay > 0 {
		dayNums = append(dayNums, domainRes.NextDay)
	}
	after, _ := a.ops.GetYAML(ctx, in.TripID)
	content, err := enrichAfterMutate(after, in.Day, dayNums, mutateResult{
		OK: true, Op: "changeOvernight", ID: res.ID, VersionID: res.VersionID, BundleOK: res.BundleOK,
		Changed: changed, DerivedChanges: derived, Preserved: preserved,
	})
	return toolResult{Content: content, Mutated: true}, err
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

func handleListPreferences(ctx context.Context, a *Agent, in TurnInput, _ string, tc *turnToolContext) (toolResult, error) {
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

func handleSavePreference(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
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

func handleForgetPreference(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
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

func handleListLearnings(ctx context.Context, a *Agent, in TurnInput, _ string, tc *turnToolContext) (toolResult, error) {
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

func handleSaveLearning(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
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

func handleForgetLearning(ctx context.Context, a *Agent, in TurnInput, argsJSON string, tc *turnToolContext) (toolResult, error) {
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
