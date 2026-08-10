package store

import (
	"context"
	"testing"
	"time"
)

func TestPreferencesKeyRejectsTraversal(t *testing.T) {
	for _, sub := range []string{"", "../x", "a/b", `a\b`} {
		if _, err := preferencesKey(sub); err == nil {
			t.Fatalf("expected error for %q", sub)
		}
	}
	if _, err := preferencesKey("user_abc-123"); err != nil {
		t.Fatal(err)
	}
}

func TestMemPreferencesRoundTrip(t *testing.T) {
	m := NewMem()
	ctx := context.Background()
	doc, err := m.GetPreferences(ctx, "sub1")
	if err != nil || len(doc.Items) != 0 {
		t.Fatalf("empty: %+v err=%v", doc, err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	in := PreferencesDoc{
		UpdatedAt: now,
		Items: []PreferenceItem{
			{ID: "pref_veg", Text: "Prefer vegetarian options", Tags: []string{"food"}, UpdatedAt: now},
		},
	}
	if err := m.PutPreferences(ctx, "sub1", in); err != nil {
		t.Fatal(err)
	}
	out, err := m.GetPreferences(ctx, "sub1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].ID != "pref_veg" || out.Items[0].Text != in.Items[0].Text {
		t.Fatalf("%+v", out)
	}
}
