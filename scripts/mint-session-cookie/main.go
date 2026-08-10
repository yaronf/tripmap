// Mint a tripmap_session cookie for local chat smoke (same HMAC as tripmapd).
//
//	HELLO_SESSION_SECRET=… go run ./scripts/mint-session-cookie -email yaronf@gmx.com
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	email := flag.String("email", "", "session email (must be on hello + chat allowlists)")
	sub := flag.String("sub", "local-smoke", "session subject")
	name := flag.String("name", "Local Smoke", "display name")
	ttl := flag.Duration("ttl", time.Hour, "cookie TTL")
	flag.Parse()

	secret := strings.TrimSpace(os.Getenv("HELLO_SESSION_SECRET"))
	if secret == "" {
		fmt.Fprintln(os.Stderr, "HELLO_SESSION_SECRET required")
		os.Exit(2)
	}
	if strings.TrimSpace(*email) == "" {
		fmt.Fprintln(os.Stderr, "-email required")
		os.Exit(2)
	}

	sess := map[string]any{
		"sub":   *sub,
		"email": *email,
		"name":  *name,
		"exp":   time.Now().Add(*ttl).Unix(),
	}
	raw, err := json.Marshal(sess)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	key := sha256.Sum256([]byte(secret))
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	fmt.Printf("tripmap_session=%s.%s\n", payload, sig)
}
