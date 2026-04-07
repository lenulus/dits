// Package logging centralizes slog setup for the dits binaries.
//
// It provides a single constructor (New) that parses level/format strings,
// opens a file sink (creating parent directories on demand), and wraps the
// underlying slog.Handler in a ContextHandler that promotes a request_id
// stored on context.Context into structured fields. The MCP middleware,
// workops methods, and the chi server all rely on this so a single
// request_id can be grepped across every log line for one tool call.
//
// stdio is sacred for dits-mcp (the MCP protocol speaks JSON-RPC over
// stdout). Callers MUST pass an explicit file path; the only opt-in to
// stdout/stderr is the literal sentinel "-".
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// LevelTrace is below slog.LevelDebug. The plan reserves it for wire-level
// payload dumps; almost never enabled in normal operation.
const LevelTrace = slog.Level(-8)

// ctxKey is the unexported type for context values managed by this package.
type ctxKey int

const (
	requestIDKey ctxKey = iota
)

// WithRequestID returns ctx with the given request ID attached. The
// ContextHandler will pick this up and emit it as a "request_id" attribute
// on every log record using that context.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext returns the request ID stored on ctx, or "".
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// Parse converts a textual level ("error","warn","info","debug","trace")
// to a slog.Level. Unknown levels return slog.LevelInfo and a non-nil error.
func Parse(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "error":
		return slog.LevelError, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "debug":
		return slog.LevelDebug, nil
	case "trace":
		return LevelTrace, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", level)
	}
}

// OpenSink resolves a textual sink target to an io.Writer.
//
// The sentinel "-" maps to stderr (callers explicitly opting out of file
// logging — never the default for dits-mcp). Any other value is treated as
// a filesystem path; parent directories are created on demand and the file
// is opened in append mode. The empty string is rejected so callers cannot
// silently fall back to stdio.
func OpenSink(target string) (io.Writer, error) {
	if target == "" {
		return nil, fmt.Errorf("log sink path is required")
	}
	if target == "-" {
		return os.Stderr, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, fmt.Errorf("creating log dir: %w", err)
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	return f, nil
}

// New constructs a *slog.Logger writing to w with the given level/format.
// Format may be "text" or "json". The handler is wrapped with a
// ContextHandler so request_id flows through context.Context.
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				lvl, ok := a.Value.Any().(slog.Level)
				if ok && lvl == LevelTrace {
					a.Value = slog.StringValue("TRACE")
				}
			}
			return a
		},
	}
	var h slog.Handler
	switch strings.ToLower(format) {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	default:
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(&ContextHandler{Handler: h})
}

// ContextHandler is a slog.Handler that pulls cross-cutting fields out of
// context.Context (currently just request_id) and adds them to every record.
type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDFromContext(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithGroup(name)}
}

// DefaultMCPLogPath returns the default file path for dits-mcp logs.
// $XDG_STATE_HOME/dits/mcp.log, falling back to ~/.dits/mcp.log.
func DefaultMCPLogPath() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "dits", "mcp.log"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".dits", "mcp.log"), nil
}

// Discard returns a logger that drops everything. Useful as a default in
// tests where logging is not under test.
func Discard() *slog.Logger {
	return slog.New(&ContextHandler{Handler: slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})})
}
