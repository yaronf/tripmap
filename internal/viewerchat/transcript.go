package viewerchat

import (
	"fmt"
	"strings"
)

const (
	// verbatimMessageCap is how many recent user/assistant messages stay full text.
	verbatimMessageCap = 8
)

type curatedTranscript struct {
	Summary  string // empty when history fits in the verbatim window
	Messages []ClientMessage
}

// curateMessages keeps the last verbatimMessageCap messages intact and builds a
// heuristic summary of older turns (no extra LLM call).
func curateMessages(msgs []ClientMessage) curatedTranscript {
	usable := make([]ClientMessage, 0, len(msgs))
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		switch role {
		case "user", "assistant":
			usable = append(usable, ClientMessage{Role: role, Content: content})
		}
	}
	usable = neutralizePriorAssistantClaims(usable)
	if len(usable) <= verbatimMessageCap {
		return curatedTranscript{Messages: usable}
	}
	older := usable[:len(usable)-verbatimMessageCap]
	recent := usable[len(usable)-verbatimMessageCap:]
	return curatedTranscript{
		Summary:  heuristicThreadSummary(older),
		Messages: recent,
	}
}

// neutralizePriorAssistantClaims replaces earlier assistant "I added/updated…" prose with a
// short history stub so the model does not reenact the previous turn when the user asks
// to remove or change something (Hayes Common remove-after-add failure mode).
func neutralizePriorAssistantClaims(msgs []ClientMessage) []ClientMessage {
	lastUser := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.ToLower(msgs[i].Role) == "user" {
			lastUser = i
			break
		}
	}
	if lastUser <= 0 {
		return msgs
	}
	out := make([]ClientMessage, len(msgs))
	copy(out, msgs)
	for i := 0; i < lastUser; i++ {
		if strings.ToLower(out[i].Role) != "assistant" {
			continue
		}
		if !looksLikeMutationClaim(out[i].Content) {
			continue
		}
		snippet := truncateRunes(out[i].Content, 80)
		out[i].Content = fmt.Sprintf(
			"[Prior turn — history only] Assistant reported an itinerary change (%s). "+
				"Do not re-announce or re-do that change. Obey the latest user message only.",
			snippet,
		)
	}
	return out
}

func heuristicThreadSummary(msgs []ClientMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		role := strings.ToLower(m.Role)
		switch role {
		case "user":
			fmt.Fprintf(&b, "- User asked: %s\n", truncateRunes(m.Content, 160))
		case "assistant":
			line := truncateRunes(m.Content, 120)
			if looksLikeMutationClaim(m.Content) || strings.HasPrefix(m.Content, "[Prior turn") {
				fmt.Fprintf(&b, "- Assistant previously reported an itinerary change: %s\n", line)
			} else {
				fmt.Fprintf(&b, "- Assistant replied: %s\n", line)
			}
		}
	}
	if b.Len() == 0 {
		return "(no prior turns)"
	}
	return strings.TrimSpace(b.String())
}

func looksLikeMutationClaim(s string) bool {
	lower := strings.ToLower(s)
	for _, needle := range []string{
		"updated", "changed", "restored", "replaced", "added", "removed",
		"patched", "overnight", "route", "photo",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
