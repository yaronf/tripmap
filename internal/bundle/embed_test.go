package bundle

import (
	"strings"
	"testing"
)

func TestViewerTypeIconsEmbedded(t *testing.T) {
	want := []string{
		"airport.svg",
		"attraction.svg",
		"depart.svg",
		"ferry.svg",
		"hut.svg",
		"overnight.svg",
		"stop.svg",
		"trailhead.svg",
		"viewpoint.svg",
	}
	for _, name := range want {
		b, err := viewerFS.ReadFile("viewer/icons/" + name)
		if err != nil {
			t.Fatalf("missing viewer/icons/%s: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("viewer/icons/%s is empty", name)
		}
	}
}

func TestViewerShellServesJSNotTripJSON(t *testing.T) {
	b, ct, ok := ViewerShell("app.js")
	if !ok || !strings.Contains(string(b), "SINGLE_LOCATION_ZOOM") {
		t.Fatalf("ViewerShell app.js ok=%v ct=%s", ok, ct)
	}
	if _, _, ok := ViewerShell("trip.json"); ok {
		t.Fatal("trip.json must not come from the viewer embed")
	}
	if _, _, ok := ViewerShell("geo/day-01.json"); ok {
		t.Fatal("geo files must not come from the viewer embed")
	}
	png, ct, ok := ViewerShell("icon.png")
	if !ok || ct != "image/png" || len(png) < 100 {
		t.Fatalf("ViewerShell icon.png ok=%v ct=%s len=%d", ok, ct, len(png))
	}
	alias, aliasCT, ok := ViewerShell("icon.svg")
	if !ok || aliasCT != "image/png" || len(alias) != len(png) {
		t.Fatalf("ViewerShell icon.svg alias ok=%v ct=%s len=%d", ok, aliasCT, len(alias))
	}
}
