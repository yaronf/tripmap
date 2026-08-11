package store

import (
	"fmt"
	"path"
	"strings"
	"time"
)

const (
	MaxLearningItems = 30
	MaxLearningText  = 500
)

// LearningItem is one durable agent operating rule for viewer chat.
type LearningItem struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LearningsDoc is users/{sub}/agent_learnings.json in the comments bucket.
type LearningsDoc struct {
	UpdatedAt time.Time      `json:"updated_at"`
	Items     []LearningItem `json:"items"`
}

func learningsKey(sub string) (string, error) {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return "", fmt.Errorf("empty user sub")
	}
	if len(sub) > 128 {
		return "", fmt.Errorf("user sub too long")
	}
	if strings.Contains(sub, "..") || strings.ContainsAny(sub, `/\`) {
		return "", fmt.Errorf("invalid user sub")
	}
	for _, r := range sub {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("invalid user sub")
		}
	}
	return path.Join("users", sub, "agent_learnings.json"), nil
}

// EmptyLearnings returns a doc with an empty items slice.
func EmptyLearnings() LearningsDoc {
	return LearningsDoc{Items: []LearningItem{}}
}
