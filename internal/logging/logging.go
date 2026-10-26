// Package logging sets up structured logging with log/slog for hlops.
// File logs are written under $HOME/.local/state/hlops/logs by default.
// Colour output is only used when stderr is an interactive terminal;
// non-interactive mode never emits ANSI escapes.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
)

var fileHandle *os.File

// Options controls logger construction.
type Options struct {
	Level   string // debug|info|warn|error
	File    string // empty => default LogDir()/hlops.log, "-" disables file log
	Quiet   bool   // suppress stderr handler entirely
	NoColor bool   // force-disable colour
	Prefix  string // extra attribute e.g. command name
}

// Setup builds the global slog logger. Returns a cleanup func closing files.
func Setup(o Options) (func(), error) {
	lvl := parseLevel(o.Level)

	var handlers []slog.Handler
	opts := &slog.HandlerOptions{Level: lvl}

	if !o.Quiet {
		handlers = append(handlers, newHandler(opts, o.NoColor))
	}

	cleanup := func() {}
	if o.File != "-" {
		file := o.File
		if file == "" {
			dir := paths.LogDir()
			if err := paths.EnsureDir(dir); err == nil {
				file = filepath.Join(dir, "hlops.log")
			}
		}
		if file != "" {
			f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				fileHandle = f
				handlers = append(handlers, slog.NewTextHandler(f, opts))
				cleanup = func() { _ = f.Close() }
			} else {
				return cleanup, fmt.Errorf("open log file: %w", err)
			}
		}
	}

	lg := slog.New(newMultiHandler(handlers...))
	if o.Prefix != "" {
		lg = lg.With("cmd", o.Prefix)
	}
	slog.SetDefault(lg)
	return cleanup, nil
}

// CloseFile closes the open log file handle, if any.
func CloseFile() {
	if fileHandle != nil {
		_ = fileHandle.Close()
		fileHandle = nil
	}
}

func newHandler(opts *slog.HandlerOptions, noColor bool) slog.Handler {
	if IsTerminal(os.Stderr) && !noColor {
		return NewColorHandler(os.Stderr, opts.Level.Level())
	}
	return slog.NewTextHandler(os.Stderr, opts)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// IsTerminal reports whether f is attached to a character device (tty).
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// --- multi handler: fan out records to several handlers ---

type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(h ...slog.Handler) slog.Handler {
	return &multiHandler{handlers: h}
}

func (m *multiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(a []slog.Attr) slog.Handler {
	nm := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		nm[i] = h.WithAttrs(a)
	}
	return &multiHandler{handlers: nm}
}

func (m *multiHandler) WithGroup(g string) slog.Handler {
	nm := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		nm[i] = h.WithGroup(g)
	}
	return &multiHandler{handlers: nm}
}

// --- simple colour text handler (only used on interactive terminals) ---

// ColorHandler writes ANSI-coloured, key=value log lines to w.
type ColorHandler struct {
	w     io.Writer
	level slog.Level
	attrs []slog.Attr
}

// NewColorHandler returns a handler that colours level strings.
func NewColorHandler(w io.Writer, lvl slog.Level) *ColorHandler {
	return &ColorHandler{w: w, level: lvl}
}

var colorFor = map[slog.Level]string{
	slog.LevelDebug: "\x1b[36m", // cyan
	slog.LevelInfo:  "\x1b[32m", // green
	slog.LevelWarn:  "\x1b[33m", // yellow
	slog.LevelError: "\x1b[31m", // red
}

func (h *ColorHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *ColorHandler) Handle(_ context.Context, r slog.Record) error {
	ts := r.Time.Format("2006-01-02T15:04:05Z07:00")
	color, ok := colorFor[r.Level]
	if !ok {
		color = colorFor[slog.LevelInfo]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "time=%s level=%s%s\x1b[0m msg=%q", ts, color, r.Level.String(), r.Message)
	for _, a := range h.attrs {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})
	b.WriteString("\n")
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *ColorHandler) WithAttrs(a []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), a...)
	return &nh
}

func (h *ColorHandler) WithGroup(_ string) slog.Handler {
	nh := *h
	return &nh
}
