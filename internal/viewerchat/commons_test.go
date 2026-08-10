package viewerchat

import (
	"context"
	"strings"
	"testing"
)

func TestResolveCommonsFile(t *testing.T) {
	ctx := context.Background()
	img, err := resolveCommonsFile(ctx, "File:Kent Street in Kingston.jpg")
	if err != nil {
		t.Fatal(err)
	}
	url := pickCommonsPhotoURL(img)
	if !strings.Contains(url, "upload.wikimedia.org") {
		t.Fatalf("url=%q", url)
	}
	if strings.Contains(url, "utm_source") {
		t.Fatalf("utm not stripped: %q", url)
	}
}

func TestSearchCommonsImages_Kingston(t *testing.T) {
	ctx := context.Background()
	imgs, err := searchCommonsImages(ctx, "Kingston New Zealand Lake Wakatipu", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) == 0 {
		t.Fatal("expected at least one image")
	}
}

func TestResolvePhotoForTrip_Query(t *testing.T) {
	ctx := context.Background()
	url, title, err := resolvePhotoForTrip(ctx, "", "Kent Street Kingston New Zealand", nil)
	if err != nil {
		t.Fatal(err)
	}
	if url == "" || title == "" {
		t.Fatalf("url=%q title=%q", url, title)
	}
	if !strings.Contains(url, "upload.wikimedia.org") {
		t.Fatalf("url=%q", url)
	}
}

func TestResolvePhotoForTrip_ExcludesCurrent(t *testing.T) {
	ctx := context.Background()
	first, title, err := resolvePhotoForTrip(ctx, "", "Kingston New Zealand", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, title2, err := resolvePhotoForTrip(ctx, "", "Kingston New Zealand", []string{first, title})
	if err != nil {
		t.Fatal(err)
	}
	if photoIdentity(first) == photoIdentity(second) {
		t.Fatalf("expected different photo, both %q / %q", title, title2)
	}
}

func TestPhotoIdentity(t *testing.T) {
	a := "https://upload.wikimedia.org/wikipedia/commons/thumb/a/a3/Kingston%2C_New_Zealand_%281%29.JPG/1920px-Kingston%2C_New_Zealand_%281%29.JPG"
	b := "File:Kingston, New Zealand (1).JPG"
	if photoIdentity(a) != photoIdentity(b) {
		t.Fatalf("%q vs %q", photoIdentity(a), photoIdentity(b))
	}
}

func TestFileTitleFromPhotoURL(t *testing.T) {
	got := fileTitleFromPhotoURL("https://upload.wikimedia.org/wikipedia/commons/1/1d/Kent_Street_in_Kingston.jpg")
	if got != "File:Kent_Street_in_Kingston.jpg" {
		t.Fatalf("got %q", got)
	}
}
