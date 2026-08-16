package viewerchat

import (
	"strings"
	"testing"
)

func TestRejectChatStructuralPatch(t *testing.T) {
	yaml := []byte(`
trip: T
schema_version: 2
places:
  a: {title: A, type: overnight}
  b: {title: B, type: overnight}
  c: {title: C, type: attraction}
days:
  - day: 1
    title: One
    route: [{place: a, type: overnight}, {place: b, type: overnight}]
`)
	err := rejectChatStructuralPatch([]byte(`{"days":{"1":{"route":[{"place":"a"},{"place":"c"}]}}}`), yaml)
	if err == nil || !strings.Contains(err.Error(), "changeOvernight") {
		t.Fatalf("err=%v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"upsert_stop":{"day":1,"list":"route","place":"c"}}`), yaml)
	if err == nil || !strings.Contains(err.Error(), "overnight") {
		t.Fatalf("err=%v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"upsert_stop":{"day":1,"list":"stops","place":"c"}}`), yaml)
	if err != nil {
		t.Fatalf("stops upsert should be allowed: %v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"update_day":{"day":1,"notes":"hi"}}`), yaml)
	if err != nil {
		t.Fatalf("update_day notes should be allowed: %v", err)
	}
}

func TestLooksLikeCancel(t *testing.T) {
	for _, s := range []string{"nm", "never mind", "You added Huka Falls, not exactly what I asked for, but NM."} {
		if !looksLikeCancel(strings.ToLower(s)) {
			t.Fatalf("expected cancel: %q", s)
		}
	}
	if looksLikeCancel("add a stop for coffee") {
		t.Fatal("should not cancel")
	}
}
