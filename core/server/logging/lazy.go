package logging

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// lazyHandler forwards to whatever slog.Default() is at the moment a record is
// logged, not at the moment the logger was built.
//
// Package loggers are usually package-level vars (var log = ForPackage("x")),
// which Go initializes before main runs Install. slog.Default().With(...)
// binds the handler at call time, so such a var would keep slog's original
// handler forever: that handler routes through the log package, which drops
// the level, the ctx (and with it Sentry user attribution) and never reaches
// Sentry at all.
//
// WithAttrs and WithGroup calls are recorded in order and replayed on the
// resolved handler, so a With/WithGroup chain on a package logger keeps its
// exact shape.
type lazyHandler struct {
	ops   []handlerOp
	cache atomic.Pointer[resolvedHandler]
}

// handlerOp is one recorded WithAttrs (group == "") or WithGroup call.
type handlerOp struct {
	attrs []slog.Attr
	group string
}

// resolvedHandler memoizes the replayed chain for one default logger. The
// key is the *slog.Logger pointer, which is always comparable (a handler
// value may not be). slog.Default() is an atomic load, so the hot path is
// one atomic load plus one pointer compare; the chain is only rebuilt after
// slog.SetDefault swaps the default.
type resolvedHandler struct {
	base    *slog.Logger
	handler slog.Handler
}

func newLazyHandler(ops []handlerOp) *lazyHandler {
	return &lazyHandler{ops: ops}
}

// resolve must never be reached with this handler installed as the default
// itself: it would recurse forever. Install builds its fan-out from concrete
// handlers only, and nothing should pass a ForPackage logger to SetDefault.
func (h *lazyHandler) resolve() slog.Handler {
	base := slog.Default()
	if c := h.cache.Load(); c != nil && c.base == base {
		return c.handler
	}
	resolved := base.Handler()
	for _, op := range h.ops {
		if op.group != "" {
			resolved = resolved.WithGroup(op.group)
		} else {
			resolved = resolved.WithAttrs(op.attrs)
		}
	}
	h.cache.Store(&resolvedHandler{base: base, handler: resolved})
	return resolved
}

func (h *lazyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.resolve().Enabled(ctx, level)
}

func (h *lazyHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.resolve().Handle(ctx, r)
}

func (h *lazyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.with(handlerOp{attrs: attrs})
}

func (h *lazyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.with(handlerOp{group: name})
}

// with copies the op list so sibling loggers derived from one parent never
// share (and overwrite) a backing array.
func (h *lazyHandler) with(op handlerOp) *lazyHandler {
	ops := make([]handlerOp, len(h.ops), len(h.ops)+1)
	copy(ops, h.ops)
	return newLazyHandler(append(ops, op))
}
