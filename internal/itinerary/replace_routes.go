package itinerary

import (
	"encoding/json"
	"fmt"
)

// ParseReplaceDayRoutes validates a replaceDayRoutes body and returns an
// equivalent Patch (places + days with required non-empty route arrays).
// Lodging ends mistyped as via/depart/empty are normalized to overnight so the
// viewer can show Depart on the morning start.
func ParseReplaceDayRoutes(body []byte) (Patch, error) {
	var req struct {
		Places map[string]any `json:"places"`
		Days   map[string]any `json:"days"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return Patch{}, fmt.Errorf("invalid replaceDayRoutes json: %w", err)
	}
	if len(req.Days) == 0 {
		return Patch{}, fmt.Errorf("replaceDayRoutes: days is required")
	}
	for key, raw := range req.Days {
		m, ok := raw.(map[string]any)
		if !ok {
			return Patch{}, fmt.Errorf("replaceDayRoutes: days.%s must be an object", key)
		}
		route, ok := m["route"]
		if !ok {
			return Patch{}, fmt.Errorf("replaceDayRoutes: days.%s.route is required (full route replacement)", key)
		}
		n, err := jsonArrayLen(route)
		if err != nil || n < 1 {
			return Patch{}, fmt.Errorf("replaceDayRoutes: days.%s.route must be a non-empty array", key)
		}
		normalizeRouteLodgingTypes(m)
	}
	return Patch{Places: req.Places, Days: req.Days}, nil
}

// normalizeRouteLodgingTypes forces first/last stops on multi-stop routes away
// from via/depart/empty (catalog via towns must still be overnight when lodging).
// Leaves ferry_terminal/airport/etc. alone. Clears bad day-local maps_url on
// those rewritten ends so place coords win for routing/pins.
func normalizeRouteLodgingTypes(day map[string]any) {
	arr, ok := day["route"].([]any)
	if !ok || len(arr) < 2 {
		return
	}
	forceLodgingRole(arr[0])
	forceLodgingRole(arr[len(arr)-1])
}

func forceLodgingRole(stop any) {
	m, ok := stop.(map[string]any)
	if !ok {
		return
	}
	t, _ := m["type"].(string)
	switch t {
	case "via", "depart", "":
		m["type"] = "overnight"
		delete(m, "maps_url")
	}
}

func jsonArrayLen(v any) (int, error) {
	switch x := v.(type) {
	case []any:
		return len(x), nil
	case nil:
		return 0, fmt.Errorf("null")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return 0, err
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(b, &arr); err != nil {
			return 0, err
		}
		return len(arr), nil
	}
}
