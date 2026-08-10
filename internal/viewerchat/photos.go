package viewerchat

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

var (
	mdImageRe       = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)
	commonsFilePath = regexp.MustCompile(`(?i)^https?://(?:commons\.wikimedia\.org|upload\.wikimedia\.org)/wiki/File:(.+)$`)
	wikiFilePath    = regexp.MustCompile(`(?i)^https?://(?:[a-z]+\.)?wikipedia\.org/wiki/File:(.+)$`)
)

// scrubAssistantMarkdownImages turns ![alt](url) into a plain URL so Persona
// does not try to render broken/untrusted images in the chat transcript.
func scrubAssistantMarkdownImages(s string) string {
	return mdImageRe.ReplaceAllString(s, "$1")
}

func photoURLsInPatch(patchJSON json.RawMessage) []string {
	var raw map[string]json.RawMessage
	if json.Unmarshal(patchJSON, &raw) != nil {
		return nil
	}
	var out []string
	if ud, ok := raw["update_day"]; ok {
		var u struct {
			Photo *string `json:"photo"`
		}
		if json.Unmarshal(ud, &u) == nil && u.Photo != nil {
			out = append(out, strings.TrimSpace(*u.Photo))
		}
	}
	if places, ok := raw["places"]; ok {
		var m map[string]map[string]any
		if json.Unmarshal(places, &m) == nil {
			for _, p := range m {
				if s, ok := p["photo"].(string); ok {
					out = append(out, strings.TrimSpace(s))
				}
			}
		}
	}
	return out
}

// rewritePhotoURLsInPatch validates every photo URL in the patch and rewrites
// them to a durable https image URL (Commons via API; others via HTTP check).
func rewritePhotoURLsInPatch(ctx context.Context, patchJSON json.RawMessage) (json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(patchJSON, &raw); err != nil {
		return nil, err
	}
	changed := false
	if ud, ok := raw["update_day"]; ok {
		var u map[string]any
		if json.Unmarshal(ud, &u) == nil {
			if s, ok := u["photo"].(string); ok && strings.TrimSpace(s) != "" {
				final, _, err := resolvePhotoForTrip(ctx, s, "", nil)
				if err != nil {
					return nil, err
				}
				if final != s {
					u["photo"] = final
					changed = true
				}
				b, err := json.Marshal(u)
				if err != nil {
					return nil, err
				}
				raw["update_day"] = b
			}
		}
	}
	if places, ok := raw["places"]; ok {
		var m map[string]map[string]any
		if json.Unmarshal(places, &m) == nil {
			for id, p := range m {
				s, ok := p["photo"].(string)
				if !ok || strings.TrimSpace(s) == "" {
					continue
				}
				final, _, err := resolvePhotoForTrip(ctx, s, "", nil)
				if err != nil {
					return nil, err
				}
				if final != s {
					p["photo"] = final
					m[id] = p
					changed = true
				}
			}
			if changed || len(m) > 0 {
				b, err := json.Marshal(m)
				if err != nil {
					return nil, err
				}
				raw["places"] = b
			}
		}
	}
	if !changed && len(photoURLsInPatch(patchJSON)) == 0 {
		return patchJSON, nil
	}
	return json.Marshal(raw)
}

func normalizePhotoCandidate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if m := commonsFilePath.FindStringSubmatch(raw); len(m) == 2 {
		return "https://commons.wikimedia.org/wiki/Special:FilePath/" + m[1]
	}
	if m := wikiFilePath.FindStringSubmatch(raw); len(m) == 2 {
		return "https://commons.wikimedia.org/wiki/Special:FilePath/" + m[1]
	}
	if u, err := url.Parse(raw); err == nil && u.Scheme == "https" {
		return raw
	}
	return raw
}

// resolveAndValidateImageURL keeps older call sites working.
func resolveAndValidateImageURL(ctx context.Context, raw string) (string, error) {
	final, _, err := resolvePhotoForTrip(ctx, raw, "", nil)
	return final, err
}
