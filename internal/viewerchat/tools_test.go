package viewerchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/yaronf/tripmap/internal/tripops"
)

type mockOps struct {
	patched             bool
	replaceDayRoutesArg string
}

func (m *mockOps) Summary(context.Context, string) (TripSummary, error) {
	return TripSummary{ID: "t", Trip: "T", Days: 1}, nil
}
func (m *mockOps) SchemaJSON(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"schema_version":2}`), nil
}
func (m *mockOps) GetYAML(context.Context, string, string, int) (YAMLResult, error) {
	return YAMLResult{Body: []byte("trip: T\ndays: []\n")}, nil
}
func (m *mockOps) GetYAMLVersion(context.Context, string, string) (YAMLResult, error) {
	return YAMLResult{Body: []byte("trip: old\n")}, nil
}
func (m *mockOps) Patch(context.Context, string, []byte) (MutateResult, error) {
	m.patched = true
	return MutateResult{ID: "t", VersionID: "v1", BundleOK: true}, nil
}
func (m *mockOps) ReplaceDayRoutes(_ context.Context, _ string, body []byte) (MutateResult, error) {
	m.replaceDayRoutesArg = string(body)
	return MutateResult{ID: "t", VersionID: "v2", BundleOK: true}, nil
}
func (m *mockOps) ListVersions(context.Context, string, int) ([]VersionEntry, error) {
	return []VersionEntry{{VersionID: "v1", IsLatest: true}}, nil
}
func (m *mockOps) RestoreVersion(context.Context, string, string) (MutateResult, error) {
	return MutateResult{ID: "t", VersionID: "v0", BundleOK: true}, nil
}
func (m *mockOps) EstimateDrive(context.Context, string, []tripops.DriveWaypoint) (tripops.DriveEstimate, error) {
	return tripops.DriveEstimate{
		Legs: []tripops.DriveLeg{{From: "a", To: "b", DistanceKm: 10, DurationMinutes: 12}},
		DistanceKm: 10, DurationMinutes: 12, Provider: "osrm",
	}, nil
}

var _ tripops.Ops = (*mockOps)(nil)

type failPatchOps struct {
	mockOps
}

func (f *failPatchOps) Patch(context.Context, string, []byte) (MutateResult, error) {
	return MutateResult{}, fmt.Errorf("invalid patch json: cannot unmarshal array into UpsertStop")
}

func TestBuildToolsIncludesChatAudienceSet(t *testing.T) {
	sess := &toolSession{
		ops:       &mockOps{},
		tripID:    "t",
		viewerDay: 1,
		log:       turnLogger{log: slog.Default(), requestID: "r", tripID: "t", sub: "s", day: 1},
	}
	tools, err := sess.buildTools()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"getSchema": true, "getTrip": true, "getTripYAML": true, "patchTrip": true,
		"replaceDayRoutes": true, "listVersions": true, "getVersion": true, "restoreVersion": true,
		"estimateDrive": true,
	}
	for _, tl := range tools {
		info, err := tl.Info(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !want[info.Name] {
			t.Fatalf("unexpected tool %q", info.Name)
		}
		delete(want, info.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing tools: %v", want)
	}
}

func TestPatchToolMarksTripUpdatedAndLogs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ops := &mockOps{}
	updated := false
	sess := &toolSession{
		ops:         ops,
		tripID:      "t",
		viewerDay:   1,
		tripUpdated: &updated,
		log:         turnLogger{log: log, requestID: "req1", tripID: "t", sub: "sub1", day: 1},
	}
	tools, err := sess.buildTools()
	if err != nil {
		t.Fatal(err)
	}
	var inv tool.InvokableTool
	for _, tl := range tools {
		info, _ := tl.Info(t.Context())
		if info.Name != "patchTrip" {
			continue
		}
		var ok bool
		inv, ok = tl.(tool.InvokableTool)
		if !ok {
			t.Fatal("patchTrip not InvokableTool")
		}
		break
	}
	if inv == nil {
		t.Fatal("patchTrip missing")
	}
	out, err := inv.InvokableRun(t.Context(), `{"patch":{"update_day":{"day":1,"title":"X"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !updated || !ops.patched {
		t.Fatalf("updated=%v patched=%v out=%s", updated, ops.patched, out)
	}
	logs := buf.String()
	if !strings.Contains(logs, `"msg":"tool_call"`) || !strings.Contains(logs, `"tool":"patchTrip"`) {
		t.Fatalf("expected tool_call log, got %s", logs)
	}
}

func TestReplaceDayRoutesFlatArgs(t *testing.T) {
	ops := &mockOps{}
	updated := false
	sess := &toolSession{
		ops:         ops,
		tripID:      "t",
		viewerDay:   5,
		tripUpdated: &updated,
		log:         turnLogger{log: slog.Default(), requestID: "r", tripID: "t", sub: "s", day: 5},
	}
	tools, err := sess.buildTools()
	if err != nil {
		t.Fatal(err)
	}
	var inv tool.InvokableTool
	for _, tl := range tools {
		info, _ := tl.Info(t.Context())
		if info.Name != "replaceDayRoutes" {
			continue
		}
		var ok bool
		inv, ok = tl.(tool.InvokableTool)
		if !ok {
			t.Fatal("not InvokableTool")
		}
		// Schema must expose days at top level, not nested body.
		if info.ParamsOneOf == nil {
			t.Fatal("missing params schema")
		}
		params, err := info.ToJSONSchema()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(params)
		if !strings.Contains(string(b), `"days"`) || strings.Contains(string(b), `"body"`) {
			t.Fatalf("want flat days, no body wrapper: %s", b)
		}
		break
	}
	if inv == nil {
		t.Fatal("replaceDayRoutes missing")
	}
	out, err := inv.InvokableRun(t.Context(), `{
		"places": {"ashhurst": {"title": "Ashhurst", "lat": -40.3, "lon": 175.7, "type": "overnight"}},
		"days": {
			"5": {"title": "To Ashhurst", "route": [{"place": "a", "type": "overnight"}, {"place": "ashhurst", "type": "overnight"}]},
			"6": {"title": "From Ashhurst", "route": [{"place": "ashhurst", "type": "overnight"}, {"place": "b", "type": "overnight"}]}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatalf("expected trip_updated, out=%s", out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(ops.replaceDayRoutesArg), &got); err != nil {
		t.Fatal(err)
	}
	days, _ := got["days"].(map[string]any)
	if days["5"] == nil || days["6"] == nil {
		t.Fatalf("expected days 5 and 6 in ops arg: %s", ops.replaceDayRoutesArg)
	}
	if got["places"] == nil {
		t.Fatalf("expected places: %s", ops.replaceDayRoutesArg)
	}
}

func TestPatchToolErrorReturnedAsResult(t *testing.T) {
	sess := &toolSession{
		ops:       &failPatchOps{},
		tripID:    "t",
		viewerDay: 1,
		log:       turnLogger{log: slog.Default(), requestID: "r", tripID: "t", sub: "s", day: 1},
	}
	tools, err := sess.buildTools()
	if err != nil {
		t.Fatal(err)
	}
	var inv tool.InvokableTool
	for _, tl := range tools {
		info, _ := tl.Info(t.Context())
		if info.Name != "patchTrip" {
			continue
		}
		var ok bool
		inv, ok = tl.(tool.InvokableTool)
		if !ok {
			t.Fatal("patchTrip not InvokableTool")
		}
		break
	}
	if inv == nil {
		t.Fatal("patchTrip missing")
	}
	out, err := inv.InvokableRun(t.Context(), `{"patch":{"upsert_stop":[]}}`)
	if err != nil {
		t.Fatalf("error handler should swallow tool error, got %v", err)
	}
	if !strings.Contains(out, `"ok":false`) || !strings.Contains(out, "cannot unmarshal") {
		t.Fatalf("expected error payload for model, got %s", out)
	}
}
