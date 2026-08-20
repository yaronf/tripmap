package httpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsersFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "users.csv")
	body := "email,sub,chat\nyaronf@gmx.com,,yes\nofra@example.com,,\n,sub_abc,true\n# comment\nOther@Example.COM,,1\nnoop@x.y,,no\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	u, err := loadUsersFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(u.LoginEmails) != 4 || u.LoginEmails[0] != "yaronf@gmx.com" || u.LoginEmails[3] != "noop@x.y" {
		t.Fatalf("login emails=%v", u.LoginEmails)
	}
	if len(u.LoginSubs) != 1 || u.LoginSubs[0] != "sub_abc" {
		t.Fatalf("login subs=%v", u.LoginSubs)
	}
	if len(u.ChatEmails) != 2 || u.ChatEmails[0] != "yaronf@gmx.com" || u.ChatEmails[1] != "other@example.com" {
		t.Fatalf("chat emails=%v", u.ChatEmails)
	}
	if len(u.ChatSubs) != 1 || u.ChatSubs[0] != "sub_abc" {
		t.Fatalf("chat subs=%v", u.ChatSubs)
	}
}
