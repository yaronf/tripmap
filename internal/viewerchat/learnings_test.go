package viewerchat

import (
	"context"
	"encoding/json"
	"testing"
)

func TestLearningToolsRoundTrip(t *testing.T) {
	ops := &memOps{}
	a := &Agent{ops: ops}
	in := TurnInput{TripID: "t1", UserSub: "sub1"}

	res, err := handleSaveLearning(context.Background(), a, in, `{"text":"Use replaceDayRoutes for overnight changes","tags":["tools"]}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		OK      bool     `json:"ok"`
		Learning Learning `json:"learning"`
	}
	if err := json.Unmarshal([]byte(res.Content), &saved); err != nil || !saved.OK || saved.Learning.Text == "" {
		t.Fatalf("%s err=%v", res.Content, err)
	}

	list, err := handleListLearnings(context.Background(), a, in, `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(list.Content)) {
		t.Fatal(list.Content)
	}

	_, err = handleForgetLearning(context.Background(), a, in, `{"learning_id":"`+saved.Learning.ID+`"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops.learnings) != 0 {
		t.Fatalf("learnings left: %+v", ops.learnings)
	}
}
