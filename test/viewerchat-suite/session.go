package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func mintSessionCookie(secret, email string) (string, error) {
	secret = strings.TrimSpace(secret)
	email = strings.TrimSpace(email)
	if secret == "" || email == "" {
		return "", fmt.Errorf("session secret and email required")
	}
	sess := map[string]any{
		"sub":   "viewerchat-suite",
		"email": email,
		"name":  "Viewerchat MT",
		"exp":   time.Now().Add(2 * time.Hour).Unix(),
	}
	raw, err := json.Marshal(sess)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	key := sha256.Sum256([]byte(secret))
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return "tripmap_session=" + payload + "." + sig, nil
}
