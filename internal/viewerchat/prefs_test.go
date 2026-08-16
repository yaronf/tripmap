package viewerchat

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPreferenceToolsRoundTrip(t *testing.T) {
	ops := &memOps{}
	a := &Agent{ops: ops}
	in := TurnInput{TripID: "t1", UserSub: "sub1"}

	res, err := handleSavePreference(context.Background(), a, in, `{"text":"Prefer vegetarian options","tags":["food"]}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		OK  bool       `json:"ok"`
		Pref Preference `json:"preference"`
	}
	if err := json.Unmarshal([]byte(res.Content), &saved); err != nil || !saved.OK || saved.Pref.Text == "" {
		t.Fatalf("%s err=%v", res.Content, err)
	}

	list, err := handleListPreferences(context.Background(), a, in, `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(list.Content)) {
		t.Fatal(list.Content)
	}

	_, err = handleForgetPreference(context.Background(), a, in, `{"preference_id":"`+saved.Pref.ID+`"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops.prefs) != 0 {
		t.Fatalf("prefs left: %+v", ops.prefs)
	}
}

func TestPreferenceToolsRequireUser(t *testing.T) {
	a := &Agent{ops: &memOps{}}
	_, err := handleSavePreference(context.Background(), a, TurnInput{TripID: "t1"}, `{"text":"x"}`, nil)
	if err == nil {
		t.Fatal("expected error without UserSub")
	}
}
