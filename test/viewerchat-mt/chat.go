package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func restoreTrip(ctx context.Context, baseURL, bearer, tripID, versionID string) error {
	body, _ := json.Marshal(map[string]string{"version_id": versionID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/agent/trips/"+tripID+"/restore", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("viewerchat-mt-restore-%d", time.Now().UnixNano()))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("restore status=%d body=%s", res.StatusCode, truncate(string(b), 400))
	}
	return nil
}

func postChat(ctx context.Context, baseURL, tripID, cookie string, history []chatMsg, day int) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"messages": history,
		"day":      day,
	})
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/me/trips/%s/api/chat", baseURL, tripID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cookie", cookie)

	client := &http.Client{Timeout: 10 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("chat status=%d body=%s", res.StatusCode, truncate(string(b), 400))
	}
	return readSSEAssistant(res.Body)
}

func readSSEAssistant(r io.Reader) (string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var text strings.Builder
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if raw == "" || raw == "[DONE]" {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Error string `json:"error"`
			Done  bool   `json:"done"`
		}
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			continue
		}
		switch {
		case ev.Type == "text" && ev.Text != "":
			text.WriteString(ev.Text)
		case ev.Type == "error" || ev.Error != "":
			return text.String(), fmt.Errorf("chat error: %s", firstNonEmpty(ev.Error, "unknown"))
		case ev.Type == "done" || ev.Done:
			return text.String(), nil
		}
	}
	if err := sc.Err(); err != nil {
		return text.String(), err
	}
	return text.String(), fmt.Errorf("SSE ended without done")
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
