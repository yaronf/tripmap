package httpserver

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

func loadHelloAllowlistFile(path string) (emails, subs []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	r.Comment = '#'
	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("allowlist header: %w", err)
	}
	colEmail, colSub := -1, -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "email":
			colEmail = i
		case "sub":
			colSub = i
		}
	}
	if colEmail < 0 && colSub < 0 {
		return nil, nil, fmt.Errorf("allowlist %s: need email and/or sub column", path)
	}

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("allowlist %s: %w", path, err)
		}
		if colEmail >= 0 && colEmail < len(rec) {
			if e := strings.ToLower(strings.TrimSpace(rec[colEmail])); e != "" {
				emails = append(emails, e)
			}
		}
		if colSub >= 0 && colSub < len(rec) {
			if s := strings.TrimSpace(rec[colSub]); s != "" {
				subs = append(subs, s)
			}
		}
	}
	return emails, subs, nil
}

func resolveAllowlistPath() string {
	if p := strings.TrimSpace(os.Getenv("HELLO_ALLOWLIST_FILE")); p != "" {
		return p
	}
	for _, p := range []string{"config/hello-allowlist.csv", "/config/hello-allowlist.csv"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "config/hello-allowlist.csv"
}
