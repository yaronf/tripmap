package viewerchat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	commonsAPI   = "https://commons.wikimedia.org/w/api.php"
	photoCheckUA = "Mozilla/5.0 (compatible; tripmap-chat/1.1; +https://tripmap.sheffer.org) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	specialFilePathRe = regexp.MustCompile(`(?i)/wiki/Special:FilePath/(.+)$`)
	uploadCommonsRe   = regexp.MustCompile(`(?i)^https?://upload\.wikimedia\.org/wikipedia/commons/(?:thumb/)?[0-9a-f]/[0-9a-f]{2}/([^/?#]+)`)
)

type commonsImage struct {
	Title    string
	URL      string
	ThumbURL string
	MIME     string
	Width    int
	Height   int
	Size     int
}

func commonsHTTPClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

func commonsGet(ctx context.Context, values url.Values) (map[string]any, error) {
	values.Set("format", "json")
	values.Set("formatversion", "2")
	u := commonsAPI + "?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", photoCheckUA)
	req.Header.Set("Accept", "application/json")
	res, err := commonsHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("commons api: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("commons api HTTP %d", res.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("commons api json: %w", err)
	}
	return raw, nil
}

func stripURLJunk(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" {
		return strings.TrimSpace(raw)
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func fileTitleFromPhotoURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := commonsFilePath.FindStringSubmatch(raw); len(m) == 2 {
		return "File:" + pathUnescape(m[1])
	}
	if m := wikiFilePath.FindStringSubmatch(raw); len(m) == 2 {
		return "File:" + pathUnescape(m[1])
	}
	if m := specialFilePathRe.FindStringSubmatch(raw); len(m) == 2 {
		name := pathUnescape(m[1])
		if !strings.HasPrefix(strings.ToLower(name), "file:") {
			name = "File:" + name
		}
		return name
	}
	if m := uploadCommonsRe.FindStringSubmatch(raw); len(m) == 2 {
		name := pathUnescape(m[1])
		// thumb paths look like 1920px-Kent_Street....jpg
		if i := strings.Index(name, "px-"); i > 0 && i <= 5 {
			name = name[i+3:]
		}
		return "File:" + name
	}
	if strings.HasPrefix(strings.ToLower(raw), "file:") {
		return raw
	}
	return ""
}

func pathUnescape(s string) string {
	out, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return out
}

func resolveCommonsFile(ctx context.Context, titleOrURL string) (commonsImage, error) {
	title := fileTitleFromPhotoURL(titleOrURL)
	if title == "" {
		title = strings.TrimSpace(titleOrURL)
		if !strings.HasPrefix(strings.ToLower(title), "file:") {
			return commonsImage{}, fmt.Errorf("not a commons file reference")
		}
	}
	vals := url.Values{}
	vals.Set("action", "query")
	vals.Set("titles", title)
	vals.Set("prop", "imageinfo")
	vals.Set("iiprop", "url|mime|size")
	vals.Set("iiurlwidth", "1600")
	raw, err := commonsGet(ctx, vals)
	if err != nil {
		return commonsImage{}, err
	}
	imgs, err := parseImageInfoPages(raw)
	if err != nil {
		return commonsImage{}, err
	}
	if len(imgs) == 0 {
		return commonsImage{}, fmt.Errorf("commons file not found: %s", title)
	}
	img := imgs[0]
	if !strings.HasPrefix(img.MIME, "image/") {
		return commonsImage{}, fmt.Errorf("commons file is not an image (%s)", img.MIME)
	}
	return img, nil
}

func searchCommonsImages(ctx context.Context, query string, limit int) ([]commonsImage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty commons search query")
	}
	if limit < 1 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	vals := url.Values{}
	vals.Set("action", "query")
	vals.Set("generator", "search")
	vals.Set("gsrsearch", query)
	vals.Set("gsrnamespace", "6")
	vals.Set("gsrlimit", fmt.Sprintf("%d", limit))
	vals.Set("prop", "imageinfo")
	vals.Set("iiprop", "url|mime|size")
	vals.Set("iiurlwidth", "1600")
	raw, err := commonsGet(ctx, vals)
	if err != nil {
		return nil, err
	}
	imgs, err := parseImageInfoPages(raw)
	if err != nil {
		return nil, err
	}
	out := make([]commonsImage, 0, len(imgs))
	for _, img := range imgs {
		if !strings.HasPrefix(img.MIME, "image/") {
			continue
		}
		if strings.Contains(img.MIME, "svg") {
			continue
		}
		titleLower := strings.ToLower(img.Title)
		// Skip obvious non-photos for day heroes.
		if strings.Contains(titleLower, "map") || strings.Contains(titleLower, "logo") || strings.Contains(titleLower, "icon") {
			continue
		}
		if img.Width > 0 && img.Height > 0 && img.Width < 640 {
			continue
		}
		out = append(out, img)
	}
	return out, nil
}

func pickCommonsPhotoURL(img commonsImage) string {
	// Prefer ~1600px thumb for viewer weight; fall back to original.
	u := strings.TrimSpace(img.ThumbURL)
	if u == "" {
		u = img.URL
	}
	return stripURLJunk(u)
}

func parseImageInfoPages(raw map[string]any) ([]commonsImage, error) {
	query, _ := raw["query"].(map[string]any)
	if query == nil {
		return nil, nil
	}
	var pages []any
	switch p := query["pages"].(type) {
	case []any:
		pages = p
	case map[string]any:
		for _, v := range p {
			pages = append(pages, v)
		}
	}
	var out []commonsImage
	for _, pageAny := range pages {
		page, _ := pageAny.(map[string]any)
		if page == nil {
			continue
		}
		if _, missing := page["missing"]; missing {
			continue
		}
		title, _ := page["title"].(string)
		infos, _ := page["imageinfo"].([]any)
		if len(infos) == 0 {
			continue
		}
		info, _ := infos[0].(map[string]any)
		if info == nil {
			continue
		}
		img := commonsImage{Title: title}
		img.URL, _ = info["url"].(string)
		img.ThumbURL, _ = info["thumburl"].(string)
		img.MIME, _ = info["mime"].(string)
		img.Width = jsonInt(info["thumbwidth"])
		if w := jsonInt(info["width"]); w > 0 {
			img.Width = w
		}
		img.Height = jsonInt(info["height"])
		if img.Height == 0 {
			img.Height = jsonInt(info["thumbheight"])
		}
		img.Size = jsonInt(info["size"])
		if img.URL == "" && img.ThumbURL == "" {
			continue
		}
		out = append(out, img)
	}
	return out, nil
}

func jsonInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case int:
		return n
	default:
		return 0
	}
}

func isWikimediaHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Host)
	return h == "commons.wikimedia.org" || h == "upload.wikimedia.org" || strings.HasSuffix(h, ".wikipedia.org")
}

func photoIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if title := fileTitleFromPhotoURL(raw); title != "" {
		t := strings.ToLower(strings.TrimPrefix(title, "File:"))
		t = strings.TrimPrefix(t, "file:")
		t = strings.ReplaceAll(t, "_", " ")
		t = strings.Join(strings.Fields(t), " ")
		return t
	}
	return strings.ToLower(stripURLJunk(raw))
}

func photoExcluded(url, title string, exclude []string) bool {
	ids := []string{photoIdentity(url), photoIdentity(title)}
	for _, ex := range exclude {
		exID := photoIdentity(ex)
		if exID == "" {
			continue
		}
		for _, id := range ids {
			if id != "" && id == exID {
				return true
			}
		}
	}
	return false
}

// resolvePhotoForTrip picks a durable https image URL.
// Wikimedia is resolved via the Commons API (ECS often cannot hotlink-check upload.wikimedia.org).
// exclude is a list of current/previous photo URLs or File: titles to skip (for "different photo").
func resolvePhotoForTrip(ctx context.Context, photo, query string, exclude []string) (string, string, error) {
	photo = strings.TrimSpace(photo)
	query = strings.TrimSpace(query)

	if photo != "" {
		if isWikimediaHost(photo) || fileTitleFromPhotoURL(photo) != "" {
			img, err := resolveCommonsFile(ctx, photo)
			if err != nil {
				return "", "", err
			}
			u := pickCommonsPhotoURL(img)
			if photoExcluded(u, img.Title, exclude) {
				return "", "", fmt.Errorf("that photo is already on the day; use query to pick a different Commons image")
			}
			return u, img.Title, nil
		}
		final, err := httpValidateImageURL(ctx, photo)
		if err != nil {
			return "", "", err
		}
		if photoExcluded(final, "", exclude) {
			return "", "", fmt.Errorf("that photo is already on the day; use query to pick a different image")
		}
		return final, "", nil
	}

	if query == "" {
		return "", "", fmt.Errorf("setDayPhoto requires photo URL or query (Commons search)")
	}
	imgs, err := searchCommonsImages(ctx, query, 20)
	if err != nil {
		return "", "", err
	}
	// Also try a category-style query for more variety when the top search hits are exhausted.
	if more, err2 := searchCommonsImages(ctx, query+" filetype:bitmap", 20); err2 == nil {
		imgs = appendCommonsUnique(imgs, more)
	}
	picked, title, ok := firstNonExcluded(imgs, exclude)
	if ok {
		return picked, title, nil
	}
	if len(imgs) == 0 {
		return "", "", fmt.Errorf("no suitable Commons images for %q", query)
	}
	return "", "", fmt.Errorf("no alternative Commons image for %q (current photo was the only match; try a more specific query)", query)
}

func appendCommonsUnique(base, more []commonsImage) []commonsImage {
	seen := map[string]bool{}
	for _, img := range base {
		seen[photoIdentity(img.Title)] = true
	}
	for _, img := range more {
		id := photoIdentity(img.Title)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		base = append(base, img)
	}
	return base
}

func firstNonExcluded(imgs []commonsImage, exclude []string) (string, string, bool) {
	for _, img := range imgs {
		u := pickCommonsPhotoURL(img)
		if photoExcluded(u, img.Title, exclude) {
			continue
		}
		return u, img.Title, true
	}
	return "", "", false
}

func httpValidateImageURL(ctx context.Context, raw string) (string, error) {
	raw = normalizePhotoCandidate(raw)
	if raw == "" {
		return "", fmt.Errorf("empty photo URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("photo must be an https URL")
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", photoCheckUA)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Range", "bytes=0-1023")
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("photo URL unreachable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("photo URL returned HTTP %d", res.StatusCode)
	}
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	if strings.Contains(ct, "text/html") {
		return "", fmt.Errorf("photo URL returned HTML, not an image")
	}
	final := raw
	if res.Request != nil && res.Request.URL != nil {
		final = res.Request.URL.String()
	}
	return stripURLJunk(final), nil
}
