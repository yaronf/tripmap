// Package mteval loads viewer-chat scenario JSON (MT + S) and scores heuristic checks.
package mteval

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Scenario is one conversation script (multi-turn MT or one-shot S).
type Scenario struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Hazard    string   `json:"hazard"`
	Setup     Setup    `json:"setup"`
	Turns     []string `json:"turns"`
	Checks    []Check  `json:"checks"`
	PassNotes string   `json:"pass_notes"`
}

// Setup restores a trip to a known version before turns.
type Setup struct {
	Trip           string `json:"trip"`
	RestoreVersion string `json:"restore_version"`
}

// Check is one heuristic assertion over turn traces.
type Check struct {
	Kind    string   `json:"kind"`
	Turn    int      `json:"turn"`
	Tools   []string `json:"tools"`
	Pattern string   `json:"pattern"`
}

// TurnTrace records one user turn's assistant text and tools.
type TurnTrace struct {
	Index     int
	User      string
	Texts     []string
	ToolNames []string
	ToolArgs  []string
}

// CheckResult is one scored check.
type CheckResult struct {
	Kind   string
	Pass   bool
	Detail string
}

// LoadScenario reads a scenario JSON file.
func LoadScenario(path string) (*Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Scenario
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.ID == "" || len(s.Turns) == 0 {
		return nil, fmt.Errorf("scenario %s: need id and turns", path)
	}
	return &s, nil
}

// EvalChecks scores checks against turn traces.
func EvalChecks(checks []Check, traces []TurnTrace) []CheckResult {
	var out []CheckResult
	allTools := func() []string {
		var names []string
		for _, t := range traces {
			names = append(names, t.ToolNames...)
		}
		return names
	}
	toolsBefore := func(turn int) []string {
		var names []string
		for _, t := range traces {
			if t.Index >= turn {
				break
			}
			names = append(names, t.ToolNames...)
		}
		return names
	}
	allText := func() string {
		var b strings.Builder
		for _, t := range traces {
			for _, x := range t.Texts {
				b.WriteString(x)
				b.WriteByte('\n')
			}
		}
		return b.String()
	}
	finalText := func() string {
		if len(traces) == 0 {
			return ""
		}
		return strings.Join(traces[len(traces)-1].Texts, "\n")
	}
	setOf := func(ss []string) map[string]bool {
		m := make(map[string]bool, len(ss))
		for _, s := range ss {
			m[s] = true
		}
		return m
	}
	intersect := func(have, want []string) []string {
		hs := setOf(have)
		var o []string
		for _, w := range want {
			if hs[w] {
				o = append(o, w)
			}
		}
		return o
	}

	for _, c := range checks {
		switch c.Kind {
		case "never_tools":
			if len(c.Tools) == 0 {
				out = append(out, CheckResult{Kind: c.Kind, Pass: true, Detail: "empty tool list (skipped)"})
				continue
			}
			found := intersect(allTools(), c.Tools)
			out = append(out, CheckResult{Kind: c.Kind, Pass: len(found) == 0,
				Detail: fmt.Sprintf("forbidden=%v found=%v", c.Tools, found)})
		case "no_tools_until_turn":
			found := toolsBefore(c.Turn)
			out = append(out, CheckResult{Kind: c.Kind, Pass: len(found) == 0,
				Detail: fmt.Sprintf("tools before turn %d: %v", c.Turn, found)})
		case "tools_must_include":
			have := setOf(allTools())
			var missing []string
			for _, t := range c.Tools {
				if !have[t] {
					missing = append(missing, t)
				}
			}
			out = append(out, CheckResult{Kind: c.Kind, Pass: len(missing) == 0,
				Detail: fmt.Sprintf("required=%v missing=%v", c.Tools, missing)})
		case "tools_after_turn_must_include":
			var after []string
			for _, t := range traces {
				if t.Index >= c.Turn {
					after = append(after, t.ToolNames...)
				}
			}
			have := setOf(after)
			var missing []string
			for _, t := range c.Tools {
				if !have[t] {
					missing = append(missing, t)
				}
			}
			out = append(out, CheckResult{Kind: c.Kind, Pass: len(missing) == 0,
				Detail: fmt.Sprintf("from turn %d required=%v missing=%v", c.Turn, c.Tools, missing)})
		case "final_text_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, CheckResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			ft := finalText()
			out = append(out, CheckResult{Kind: c.Kind, Pass: re.MatchString(ft),
				Detail: fmt.Sprintf("pattern=%q matched=%v", c.Pattern, re.MatchString(ft))})
		case "final_text_not_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, CheckResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			ft := finalText()
			out = append(out, CheckResult{Kind: c.Kind, Pass: !re.MatchString(ft),
				Detail: fmt.Sprintf("pattern=%q matched=%v", c.Pattern, re.MatchString(ft))})
		case "any_text_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, CheckResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			at := allText()
			out = append(out, CheckResult{Kind: c.Kind, Pass: re.MatchString(at),
				Detail: fmt.Sprintf("pattern=%q matched=%v", c.Pattern, re.MatchString(at))})
		case "patch_args_not_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, CheckResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			s := mutateArgs(traces)
			matched := re.MatchString(s)
			out = append(out, CheckResult{Kind: c.Kind, Pass: !matched,
				Detail: fmt.Sprintf("pattern=%q matched_in_mutate_args=%v", c.Pattern, matched)})
		case "patch_args_regex":
			re, err := regexp.Compile(c.Pattern)
			if err != nil {
				out = append(out, CheckResult{Kind: c.Kind, Pass: false, Detail: err.Error()})
				continue
			}
			s := mutateArgs(traces)
			matched := re.MatchString(s)
			out = append(out, CheckResult{Kind: c.Kind, Pass: matched,
				Detail: fmt.Sprintf("pattern=%q matched_in_mutate_args=%v", c.Pattern, matched)})
		default:
			out = append(out, CheckResult{Kind: c.Kind, Pass: false, Detail: "unknown check kind"})
		}
	}
	return out
}

func mutateArgs(traces []TurnTrace) string {
	var mutArgs strings.Builder
	for _, t := range traces {
		for i, name := range t.ToolNames {
			if name == "patchTrip" || name == "replaceDayRoutes" || name == "restoreVersion" {
				if i < len(t.ToolArgs) {
					mutArgs.WriteString(t.ToolArgs[i])
					mutArgs.WriteByte('\n')
				}
			}
		}
	}
	return mutArgs.String()
}

// AllPassed reports whether every check passed.
func AllPassed(results []CheckResult) bool {
	for _, r := range results {
		if !r.Pass {
			return false
		}
	}
	return true
}
