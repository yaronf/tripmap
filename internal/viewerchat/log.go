package viewerchat

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"
)

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmtTimeID()
	}
	return hex.EncodeToString(b[:])
}

func fmtTimeID() string {
	return time.Now().UTC().Format("150405.000")
}

func truncateRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

type turnLogger struct {
	log       *slog.Logger
	requestID string
	tripID    string
	sub       string
	day       int
}

func (t turnLogger) with(attrs ...any) *slog.Logger {
	base := []any{
		"component", "viewerchat",
		"request_id", t.requestID,
		"trip_id", t.tripID,
		"sub", t.sub,
		"day", t.day,
	}
	return t.log.With(append(base, attrs...)...)
}
