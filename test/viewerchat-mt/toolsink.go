package main

import (
	"context"
	"log/slog"
	"sync"
)

// toolSink captures viewerchat tool_call slog records for the active turn.
type toolSink struct {
	mu    sync.Mutex
	names []string
	args  []string
}

func newToolSink() *toolSink {
	return &toolSink{}
}

func (t *toolSink) BeginTurn() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.names = nil
	t.args = nil
}

func (t *toolSink) EndTurn() (names, args []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	names = append([]string(nil), t.names...)
	args = append([]string(nil), t.args...)
	return names, args
}

func (t *toolSink) record(name, args string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.names = append(t.names, name)
	t.args = append(t.args, args)
}

type teeHandler struct {
	next  slog.Handler
	sink  *toolSink
	attrs []slog.Attr
}

func (t *toolSink) Wrap(next slog.Handler) slog.Handler {
	return &teeHandler{next: next, sink: t}
}

func (h *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "tool_call" {
		merged := map[string]string{}
		for _, a := range h.attrs {
			merged[a.Key] = a.Value.String()
		}
		r.Attrs(func(a slog.Attr) bool {
			merged[a.Key] = a.Value.String()
			return true
		})
		if name := merged["tool"]; name != "" {
			h.sink.record(name, merged["args"])
		}
	}
	return h.next.Handle(ctx, r)
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := append([]slog.Attr(nil), h.attrs...)
	cp = append(cp, attrs...)
	return &teeHandler{next: h.next.WithAttrs(attrs), sink: h.sink, attrs: cp}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{next: h.next.WithGroup(name), sink: h.sink, attrs: h.attrs}
}
