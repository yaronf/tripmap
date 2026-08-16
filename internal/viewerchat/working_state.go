package viewerchat

import (
	"encoding/json"
	"strings"
)

// WorkingState is structured thread memory injected each turn (not lossy prose summary alone).
type WorkingState struct {
	ActiveIntent      map[string]any `json:"active_intent,omitempty"`
	Constraints       map[string]any `json:"constraints,omitempty"`
	RecentCorrections []string       `json:"recent_corrections,omitempty"`
	LastMutation      map[string]any `json:"last_mutation,omitempty"`
	CancelThisTurn    bool           `json:"cancel_this_turn,omitempty"`
}

func buildWorkingState(msgs []ClientMessage, lastMutateJSON string) WorkingState {
	ws := WorkingState{}
	if lastMutateJSON != "" {
		var raw map[string]any
		if json.Unmarshal([]byte(lastMutateJSON), &raw) == nil {
			ws.LastMutation = map[string]any{
				"op":         raw["op"],
				"version_id": raw["version_id"],
				"ok":         raw["ok"],
			}
		}
	}
	// Latest user message drives cancel / soft intent.
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.ToLower(msgs[i].Role) != "user" {
			continue
		}
		text := strings.TrimSpace(msgs[i].Content)
		lower := strings.ToLower(text)
		if looksLikeCancel(lower) {
			ws.CancelThisTurn = true
			ws.Constraints = map[string]any{"do_not": []string{"mutate_until_new_clear_ask"}}
			return ws
		}
		ws.ActiveIntent = map[string]any{"user_ask": truncateRunes(text, 160)}
		break
	}
	// Soft corrections from recent user turns.
	for _, m := range msgs {
		if strings.ToLower(m.Role) != "user" {
			continue
		}
		lower := strings.ToLower(m.Content)
		if strings.Contains(lower, "don't") || strings.Contains(lower, "do not") ||
			strings.Contains(lower, "wrong") || strings.Contains(lower, "messed") {
			ws.RecentCorrections = append(ws.RecentCorrections, truncateRunes(m.Content, 120))
		}
	}
	if len(ws.RecentCorrections) > 3 {
		ws.RecentCorrections = ws.RecentCorrections[len(ws.RecentCorrections)-3:]
	}
	return ws
}

func looksLikeCancel(lower string) bool {
	// Short cancel / never-mind utterances.
	trimmed := strings.TrimSpace(lower)
	trimmed = strings.Trim(trimmed, ".!,")
	switch trimmed {
	case "nm", "n/m", "never mind", "nevermind", "stop", "cancel", "don't", "do not":
		return true
	}
	if strings.HasPrefix(trimmed, "nm ") || strings.HasPrefix(trimmed, "never mind") {
		return true
	}
	// "… but NM" / "not exactly what I asked for, but NM"
	if strings.Contains(trimmed, ", but nm") || strings.HasSuffix(trimmed, " nm") ||
		strings.Contains(trimmed, " but nm") {
		return true
	}
	return false
}

var mutateToolNames = map[string]bool{
	"patchTrip": true, "replaceDayRoutes": true, "changeOvernight": true,
	"restoreVersion": true, "setDayPhoto": true,
	"savePreference": true, "forgetPreference": true,
	"saveLearning": true, "forgetLearning": true,
}

func isMutateTool(name string) bool {
	return mutateToolNames[name]
}
