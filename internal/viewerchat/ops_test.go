package viewerchat

import "testing"

func TestDayDetailFromYAML(t *testing.T) {
	body := []byte(`
trip: T
schema_version: 2
start: 2026-01-01
places:
  a:
    title: Alpha
    type: town
days:
  - day: 1
    title: One
    notes: hello
    stops:
      - place: a
`)
	d, err := DayDetailFromYAML(body, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "One" || len(d.Stops) != 1 || d.Stops[0].Title != "Alpha" {
		t.Fatalf("%+v", d)
	}
}

func TestToolHandlersCoverOpenAPI(t *testing.T) {
	h := toolHandlers()
	for _, name := range []string{
		"getSchema", "getTrip", "getTripYAML", "setDayPhoto",
		"listVersions", "getVersion", "restoreVersion", "patchTrip", "replaceDayRoutes",
		"listPreferences", "savePreference", "forgetPreference",
	} {
		if h[name] == nil {
			t.Fatalf("missing handler %s", name)
		}
	}
}
