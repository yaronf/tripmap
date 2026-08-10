package viewerchat

import (
	"regexp"
	"strings"
)

var (
	// OpenAI web_search url citations often look like [Title](https://…?utm_source=openai)
	openaiCiteRe = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]*utm_source=openai[^)]*)\)`)
	mdLinkRe     = regexp.MustCompile(`\[([^\]]*)\]\((https?://[^)]+)\)`)
	uploadWikiRe = regexp.MustCompile(`https?://upload\.wikimedia\.org/\S+`)
	wsCollapse   = regexp.MustCompile(`[ \t]{2,}`)
	// Common GPT closing fluff (applied case-insensitively).
	fillerCloseRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s*If you need more information or want to adjust any stops, let me know!?\.?\s*$`),
		regexp.MustCompile(`(?i)\s*If you need any (further|more) (help|assistance|adjustments)[^.?!]*[.?!]\s*$`),
		regexp.MustCompile(`(?i)\s*If you need[^.?!]*(let me know|feel free)[^.?!]*[.?!]\s*$`),
		regexp.MustCompile(`(?i)\s*If (there's|there is|you('d| would)? like) anything else[^.?!]*[.?!]\s*$`),
		regexp.MustCompile(`(?i)\s*Let me know if (there's|there is|you need|you'd like|you would like)[^.?!]*[.?!]\s*$`),
		regexp.MustCompile(`(?i)\s*(Just )?let me know[!?.]*\s*$`),
		regexp.MustCompile(`(?i)\s*Feel free to (let me know|ask)[^.?!]*[.?!]\s*$`),
	}
)

// cleanAssistantText removes web_search citation spam and collapses accidental
// exact-duplicate sentence/paragraph repeats from Responses OutputText().
func cleanAssistantText(s string) string {
	s = scrubAssistantMarkdownImages(s)
	s = openaiCiteRe.ReplaceAllString(s, "")
	s = uploadWikiRe.ReplaceAllString(s, "")
	// Drop bare markdown links that are only a site title (common citation shape)
	// when they sit flush against the next sentence with no space — leave normal
	// intentional links that have surrounding whitespace alone for now.
	s = mdLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdLinkRe.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		title, link := sub[1], sub[2]
		if strings.Contains(link, "utm_source=openai") || strings.Contains(link, "upload.wikimedia.org") {
			return ""
		}
		// Keep short intentional links; drop long "Article Title | Site" citation labels.
		if strings.Contains(title, "|") || len(title) > 48 {
			return ""
		}
		return m
	})
	s = wsCollapse.ReplaceAllString(s, " ")
	s = collapseRepeatedChunks(s)
	s = stripFillerClosings(s)
	return strings.TrimSpace(s)
}

func stripFillerClosings(s string) string {
	s = strings.TrimSpace(s)
	for {
		next := s
		for _, re := range fillerCloseRes {
			next = strings.TrimSpace(re.ReplaceAllString(next, ""))
		}
		if next == s {
			return s
		}
		s = next
	}
}

func collapseRepeatedChunks(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// Exact full-string repeats glued together (common when OutputText concatenates
	// the same message + citation block N times). Prefer the smallest period.
	if chunk, ok := smallestRepeatUnit(s); ok {
		s = chunk
	}
	// Also strip leftover bare OpenAI citation URLs glued to prose.
	s = regexp.MustCompile(`https?://\S*utm_source=openai\S*`).ReplaceAllString(s, "")
	s = wsCollapse.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	// Sentence-level: collapse consecutive identical sentences.
	parts := splitSentences(s)
	if len(parts) < 2 {
		return s
	}
	var out []string
	for i := 0; i < len(parts); {
		j := i + 1
		for j < len(parts) && strings.TrimSpace(parts[j]) == strings.TrimSpace(parts[i]) {
			j++
		}
		out = append(out, strings.TrimSpace(parts[i]))
		i = j
	}
	return strings.Join(out, " ")
}

func smallestRepeatUnit(s string) (string, bool) {
	n := len(s)
	if n < 2 {
		return "", false
	}
	for period := 1; period <= n/2; period++ {
		if n%period != 0 {
			continue
		}
		ok := true
		chunk := s[:period]
		for i := period; i < n; i += period {
			if s[i:i+period] != chunk {
				ok = false
				break
			}
		}
		if ok && n/period >= 2 {
			return strings.TrimSpace(chunk), true
		}
	}
	return "", false
}

func splitSentences(s string) []string {
	// Split on ". " / "! " / "? " while keeping the terminator on the left piece.
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if (s[i] == '.' || s[i] == '!' || s[i] == '?') && i+1 < len(s) && s[i+1] == ' ' {
			parts = append(parts, s[start:i+1])
			start = i + 2
			i++
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}
