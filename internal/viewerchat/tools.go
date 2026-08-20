package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type toolSession struct {
	ops         TripOps
	tripID      string
	viewerDay   int
	tripUpdated *bool
	log         turnLogger
}

func (s *toolSession) buildTools() ([]tool.BaseTool, error) {
	getSchema, err := utils.InferTool("getSchema", "Itinerary schema and version metadata.",
		func(ctx context.Context, _ struct{}) (json.RawMessage, error) {
			return typedWrap(s, "getSchema", "{}", false, func() (json.RawMessage, error) {
				return s.ops.SchemaJSON(ctx)
			})
		})
	if err != nil {
		return nil, err
	}
	getTrip, err := utils.InferTool("getTrip", "Compact trip title card (not full routes).",
		func(ctx context.Context, _ struct{}) (TripCard, error) {
			return typedWrap(s, "getTrip", "{}", false, func() (TripCard, error) {
				return s.ops.Summary(ctx, s.tripID)
			})
		})
	if err != nil {
		return nil, err
	}
	type yamlIn struct {
		Scope string `json:"scope" jsonschema:"description=day for neighborhood YAML or full for entire itinerary,enum=day,full"`
		Day   int    `json:"day" jsonschema:"description=Center day for scope=day (defaults to viewer day)"`
	}
	getTripYAML, err := utils.InferTool("getTripYAML", "Get itinerary YAML. Prefer scope=day for the current-day neighborhood.",
		func(ctx context.Context, in yamlIn) (map[string]any, error) {
			args, _ := json.Marshal(in)
			return typedWrap(s, "getTripYAML", string(args), false, func() (map[string]any, error) {
				scope := strings.ToLower(strings.TrimSpace(in.Scope))
				if scope == "" {
					scope = "day"
				}
				day := in.Day
				if day < 1 {
					day = s.viewerDay
				}
				body, err := s.ops.GetYAML(ctx, s.tripID, scope, day)
				if err != nil {
					return nil, err
				}
				return map[string]any{"yaml": string(body), "scope": scope, "day": day}, nil
			})
		})
	if err != nil {
		return nil, err
	}
	type patchIn struct {
		Patch json.RawMessage `json:"patch" jsonschema:"required,description=TripPatch JSON object (update_day, places, upsert_stop, etc.)"`
	}
	patchTrip, err := utils.InferTool("patchTrip",
		"Patch places info, day narrative, or structure. "+
			"Add a stop with places + upsert_stop (list route|stops). "+
			"update_day is title/notes/hike/ferry/photo only — never route/stops. "+
			"Overnight/endpoints: replaceDayRoutes.",
		func(ctx context.Context, in patchIn) (MutateResult, error) {
			args := string(in.Patch)
			return typedWrap(s, "patchTrip", args, true, func() (MutateResult, error) {
				if len(in.Patch) == 0 {
					return MutateResult{}, fmt.Errorf("patch is required")
				}
				return s.ops.Patch(ctx, s.tripID, in.Patch)
			})
		})
	if err != nil {
		return nil, err
	}
	type routesIn struct {
		Body json.RawMessage `json:"body" jsonschema:"required,description=replaceDayRoutes request JSON (days + optional places)"`
	}
	replaceDayRoutes, err := utils.InferTool("replaceDayRoutes", "Replace full day routes (overnight / endpoint changes).",
		func(ctx context.Context, in routesIn) (MutateResult, error) {
			args := string(in.Body)
			return typedWrap(s, "replaceDayRoutes", args, true, func() (MutateResult, error) {
				if len(in.Body) == 0 {
					return MutateResult{}, fmt.Errorf("body is required")
				}
				return s.ops.ReplaceDayRoutes(ctx, s.tripID, in.Body)
			})
		})
	if err != nil {
		return nil, err
	}
	listVersions, err := utils.InferTool("listVersions", "List recent YAML versions for this trip.",
		func(ctx context.Context, _ struct{}) ([]VersionEntry, error) {
			return typedWrap(s, "listVersions", "{}", false, func() ([]VersionEntry, error) {
				return s.ops.ListVersions(ctx, s.tripID)
			})
		})
	if err != nil {
		return nil, err
	}
	type verIn struct {
		VersionID string `json:"version_id" jsonschema:"required,description=Prior version id from listVersions"`
	}
	getVersion, err := utils.InferTool("getVersion", "Read-only YAML for a prior version_id.",
		func(ctx context.Context, in verIn) (map[string]any, error) {
			args, _ := json.Marshal(in)
			return typedWrap(s, "getVersion", string(args), false, func() (map[string]any, error) {
				body, err := s.ops.GetYAMLVersion(ctx, s.tripID, in.VersionID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"yaml": string(body), "version_id": strings.TrimSpace(in.VersionID)}, nil
			})
		})
	if err != nil {
		return nil, err
	}
	restoreVersion, err := utils.InferTool("restoreVersion", "Restore TO version_id (makes that revision current). To undo latest, pass previous non-latest id.",
		func(ctx context.Context, in verIn) (MutateResult, error) {
			args, _ := json.Marshal(in)
			return typedWrap(s, "restoreVersion", string(args), true, func() (MutateResult, error) {
				return s.ops.RestoreVersion(ctx, s.tripID, in.VersionID)
			})
		})
	if err != nil {
		return nil, err
	}

	// Tool failures become string results for the model so it can fix args and
	// retry in the same turn (instead of aborting the SSE stream).
	raw := []tool.BaseTool{
		getSchema, getTrip, getTripYAML, patchTrip, replaceDayRoutes, listVersions, getVersion, restoreVersion,
	}
	out := make([]tool.BaseTool, 0, len(raw))
	for _, t := range raw {
		out = append(out, utils.WrapToolWithErrorHandler(t, toolErrorForModel))
	}
	return out, nil
}

func toolErrorForModel(_ context.Context, err error) string {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "unknown tool error"
	}
	b, mErr := json.Marshal(map[string]any{
		"ok":    false,
		"error": msg,
	})
	if mErr != nil {
		return `{"ok":false,"error":` + strconv.Quote(msg) + `}`
	}
	return string(b)
}

func typedWrap[T any](s *toolSession, name, argsJSON string, mutate bool, fn func() (T, error)) (T, error) {
	var zero T
	start := time.Now()
	out, err := fn()
	latency := time.Since(start).Milliseconds()
	resultBytes := 0
	if err == nil {
		if b, mErr := json.Marshal(out); mErr == nil {
			resultBytes = len(b)
		}
	}
	attrs := []any{
		"tool", name,
		"latency_ms", latency,
		"args", truncateRunes(argsJSON, 500),
		"result_bytes", resultBytes,
		"mutate", mutate,
	}
	if err != nil {
		s.log.with(attrs...).Error("tool_call", "ok", false, "error", truncateRunes(err.Error(), 240))
		return zero, err
	}
	if mutate && s.tripUpdated != nil {
		*s.tripUpdated = true
	}
	s.log.with(attrs...).Info("tool_call", "ok", true)
	return out, nil
}
