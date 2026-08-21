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
	"github.com/yaronf/tripmap/internal/tripops"
)

// Chat binder concerns only: fixed tripID, viewer-day defaults, logging,
// trip_updated, and error-to-model. Itinerary logic lives in tripops.

const maxChatVersions = 25

type toolSession struct {
	ops         Ops
	tripID      string
	viewerDay   int
	tripUpdated *bool
	log         turnLogger
}

func (s *toolSession) buildTools() ([]tool.BaseTool, error) {
	getSchema, err := utils.InferTool("getSchema", tripops.SummaryGetSchema,
		func(ctx context.Context, _ struct{}) (json.RawMessage, error) {
			return typedWrap(s, "getSchema", "{}", false, func() (json.RawMessage, error) {
				return s.ops.SchemaJSON(ctx)
			})
		})
	if err != nil {
		return nil, err
	}
	getTrip, err := utils.InferTool("getTrip", tripops.SummaryGetTrip,
		func(ctx context.Context, _ struct{}) (TripSummary, error) {
			return typedWrap(s, "getTrip", "{}", false, func() (TripSummary, error) {
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
	getTripYAML, err := utils.InferTool("getTripYAML", tripops.SummaryGetTripYAML,
		func(ctx context.Context, in yamlIn) (map[string]any, error) {
			args, _ := json.Marshal(in)
			return typedWrap(s, "getTripYAML", string(args), false, func() (map[string]any, error) {
				// Viewer default: day neighborhood (HTTP/MCP omit → full).
				scope := strings.ToLower(strings.TrimSpace(in.Scope))
				if scope == "" {
					scope = "day"
				}
				day := in.Day
				if day < 1 {
					day = s.viewerDay
				}
				res, err := s.ops.GetYAML(ctx, s.tripID, scope, day)
				if err != nil {
					return nil, err
				}
				return map[string]any{"yaml": string(res.Body), "scope": scope, "day": day}, nil
			})
		})
	if err != nil {
		return nil, err
	}
	type patchIn struct {
		Patch json.RawMessage `json:"patch" jsonschema:"required,description=TripPatch JSON object"`
	}
	patchTrip, err := utils.InferTool("patchTrip", tripops.SummaryPatchTrip,
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
		Body json.RawMessage `json:"body" jsonschema:"required,description=replaceDayRoutes request JSON"`
	}
	replaceDayRoutes, err := utils.InferTool("replaceDayRoutes", tripops.SummaryReplaceDayRoutes,
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
	listVersions, err := utils.InferTool("listVersions", tripops.SummaryListVersions,
		func(ctx context.Context, _ struct{}) ([]VersionEntry, error) {
			return typedWrap(s, "listVersions", "{}", false, func() ([]VersionEntry, error) {
				return s.ops.ListVersions(ctx, s.tripID, maxChatVersions)
			})
		})
	if err != nil {
		return nil, err
	}
	type verIn struct {
		VersionID string `json:"version_id" jsonschema:"required,description=Prior version id from listVersions"`
	}
	getVersion, err := utils.InferTool("getVersion", tripops.SummaryGetVersion,
		func(ctx context.Context, in verIn) (map[string]any, error) {
			args, _ := json.Marshal(in)
			return typedWrap(s, "getVersion", string(args), false, func() (map[string]any, error) {
				res, err := s.ops.GetYAMLVersion(ctx, s.tripID, in.VersionID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"yaml": string(res.Body), "version_id": strings.TrimSpace(in.VersionID)}, nil
			})
		})
	if err != nil {
		return nil, err
	}
	restoreVersion, err := utils.InferTool("restoreVersion", tripops.SummaryRestoreVersion,
		func(ctx context.Context, in verIn) (MutateResult, error) {
			args, _ := json.Marshal(in)
			return typedWrap(s, "restoreVersion", string(args), true, func() (MutateResult, error) {
				return s.ops.RestoreVersion(ctx, s.tripID, in.VersionID)
			})
		})
	if err != nil {
		return nil, err
	}

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
