package mteval

import "testing"

func TestEvalChecksNeverTools(t *testing.T) {
	traces := []TurnTrace{
		{Index: 0, ToolNames: []string{"getTrip"}},
		{Index: 1, ToolNames: []string{"patchTrip"}, ToolArgs: []string{`{"update_day":{}}`}},
	}
	res := EvalChecks([]Check{
		{Kind: "never_tools", Tools: []string{"restoreVersion"}},
		{Kind: "tools_must_include", Tools: []string{"patchTrip"}},
		{Kind: "no_tools_until_turn", Turn: 0},
	}, traces)
	if !AllPassed(res) {
		t.Fatalf("%+v", res)
	}
}

func TestEvalChecksPatchArgs(t *testing.T) {
	traces := []TurnTrace{
		{Index: 0, ToolNames: []string{"patchTrip"}, ToolArgs: []string{`{"update_day":{"title":"Alpha"}}`}},
	}
	res := EvalChecks([]Check{
		{Kind: "patch_args_not_regex", Pattern: "(?i)italy"},
	}, traces)
	if !AllPassed(res) {
		t.Fatalf("%+v", res)
	}
	res = EvalChecks([]Check{
		{Kind: "patch_args_not_regex", Pattern: "(?i)alpha"},
	}, traces)
	if AllPassed(res) {
		t.Fatal("expected fail on alpha")
	}
	res = EvalChecks([]Check{
		{Kind: "patch_args_regex", Pattern: "(?i)alpha"},
	}, traces)
	if !AllPassed(res) {
		t.Fatalf("expected match on alpha: %+v", res)
	}
	res = EvalChecks([]Check{
		{Kind: "patch_args_regex", Pattern: "(?i)delta"},
	}, traces)
	if AllPassed(res) {
		t.Fatal("expected miss on delta")
	}
}
