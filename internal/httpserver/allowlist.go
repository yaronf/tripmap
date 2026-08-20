package httpserver

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

// usersFile holds Hellō sign-in ACL and optional chat ACL from one CSV.
type usersFile struct {
	LoginEmails []string
	LoginSubs   []string
	ChatEmails  []string
	ChatSubs    []string
}

// loadUsersFile reads config/users.csv (or path).
// Columns: email, sub, chat (optional). Every row with email and/or sub may sign in.
// chat=yes|true|1 grants viewer chat; empty/no/false = sign-in only.
func loadUsersFile(path string) (usersFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return usersFile{}, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	r.Comment = '#'
	header, err := r.Read()
	if err != nil {
		return usersFile{}, fmt.Errorf("users header: %w", err)
	}
	colEmail, colSub, colChat := -1, -1, -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "email":
			colEmail = i
		case "sub":
			colSub = i
		case "chat":
			colChat = i
		}
	}
	if colEmail < 0 && colSub < 0 {
		return usersFile{}, fmt.Errorf("users %s: need email and/or sub column", path)
	}

	var out usersFile
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return usersFile{}, fmt.Errorf("users %s: %w", path, err)
		}
		email, sub := "", ""
		if colEmail >= 0 && colEmail < len(rec) {
			email = strings.ToLower(strings.TrimSpace(rec[colEmail]))
		}
		if colSub >= 0 && colSub < len(rec) {
			sub = strings.TrimSpace(rec[colSub])
		}
		if email == "" && sub == "" {
			continue
		}
		if email != "" {
			out.LoginEmails = append(out.LoginEmails, email)
		}
		if sub != "" {
			out.LoginSubs = append(out.LoginSubs, sub)
		}
		chat := false
		if colChat >= 0 && colChat < len(rec) {
			chat = truthyChat(rec[colChat])
		}
		if chat {
			if email != "" {
				out.ChatEmails = append(out.ChatEmails, email)
			}
			if sub != "" {
				out.ChatSubs = append(out.ChatSubs, sub)
			}
		}
	}
	return out, nil
}

func truthyChat(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "chat":
		return true
	default:
		return false
	}
}

func resolveUsersPath() string {
	if p := strings.TrimSpace(os.Getenv("USERS_FILE")); p != "" {
		return p
	}
	// Legacy env names still override path (same unified file).
	if p := strings.TrimSpace(os.Getenv("HELLO_ALLOWLIST_FILE")); p != "" {
		return p
	}
	for _, p := range []string{"config/users.csv", "/config/users.csv"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "config/users.csv"
}
