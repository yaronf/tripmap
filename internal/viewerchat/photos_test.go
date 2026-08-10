package viewerchat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestScrubAssistantMarkdownImages(t *testing.T) {
	in := "Here: ![Kingston](https://upload.wikimedia.org/wikipedia/commons/x.jpg) done"
	got := scrubAssistantMarkdownImages(in)
	want := "Here: https://upload.wikimedia.org/wikipedia/commons/x.jpg done"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPhotoURLsInPatch(t *testing.T) {
	raw := json.RawMessage(`{
		"update_day":{"day":22,"photo":"https://example.com/a.jpg","photo_caption":"A"},
		"places":{"kingston":{"photo":"https://example.com/b.jpg"}}
	}`)
	got := photoURLsInPatch(raw)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestNormalizePhotoCandidate(t *testing.T) {
	got := normalizePhotoCandidate("https://commons.wikimedia.org/wiki/File:Lake_Wakatipu.NZ_(22233131935).jpg")
	if !strings.Contains(got, "Special:FilePath/") {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAndValidateImageURL_WikimediaViaAPI(t *testing.T) {
	ctx := context.Background()
	final, err := resolveAndValidateImageURL(ctx, "https://commons.wikimedia.org/wiki/File:Kent_Street_in_Kingston.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(final, "upload.wikimedia.org") {
		t.Fatalf("expected upload.wikimedia.org, got %q", final)
	}
}

func TestResolveAndValidateImageURL_404(t *testing.T) {
	ctx := context.Background()
	_, err := resolveAndValidateImageURL(ctx, "https://www.queenstown.com/wp-content/uploads/2020/09/68348165_856990800756114_7933008045884306176_n.jpg")
	if err == nil {
		t.Fatal("expected error for 404 tourism URL")
	}
}

func TestRewritePhotoURLsInPatch(t *testing.T) {
	ctx := context.Background()
	raw := json.RawMessage(`{"update_day":{"day":22,"photo":"https://commons.wikimedia.org/wiki/File:Kent_Street_in_Kingston.jpg","photo_caption":"Lake"}}`)
	out, err := rewritePhotoURLsInPatch(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		UpdateDay struct {
			Photo string `json:"photo"`
		} `json:"update_day"`
	}
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.UpdateDay.Photo, "upload.wikimedia.org") {
		t.Fatalf("photo not rewritten: %q", p.UpdateDay.Photo)
	}
}
