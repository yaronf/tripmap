package store

import (
	"context"
	"testing"
	"time"
)

func TestLearningsKeyRejectsBadSub(t *testing.T) {
	for _, sub := range []string{"", "../x", "a/b", "a\\b"} {
		if _, err := learningsKey(sub); err == nil {
			t.Fatalf("expected error for %q", sub)
		}
	}
	if _, err := learningsKey("user_abc-123"); err != nil {
		t.Fatal(err)
	}
}

func TestMemLearningsRoundTrip(t *testing.T) {
	m := NewMem()
	ctx := context.Background()
	doc, err := m.GetLearnings(ctx, "sub1")
	if err != nil || len(doc.Items) != 0 {
		t.Fatalf("%+v %v", doc, err)
	}
	in := LearningsDoc{
		UpdatedAt: time.Now().UTC(),
		Items: []LearningItem{{
			ID: "learn_1", Text: "Use replaceDayRoutes for overnight changes", UpdatedAt: time.Now().UTC(),
		}},
	}
	if err := m.PutLearnings(ctx, "sub1", in); err != nil {
		t.Fatal(err)
	}
	out, err := m.GetLearnings(ctx, "sub1")
	if err != nil || len(out.Items) != 1 || out.Items[0].ID != "learn_1" {
		t.Fatalf("%+v %v", out, err)
	}
}
