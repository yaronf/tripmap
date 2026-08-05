package httpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHelloAllowlistFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.csv")
	body := "email,sub\nyaronf@gmx.com,\n,sub_abc\n# comment row ignored via Comment\nOther@Example.COM,\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	emails, subs, err := loadHelloAllowlistFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(emails) != 2 || emails[0] != "yaronf@gmx.com" || emails[1] != "other@example.com" {
		t.Fatalf("emails=%v", emails)
	}
	if len(subs) != 1 || subs[0] != "sub_abc" {
		t.Fatalf("subs=%v", subs)
	}
}
