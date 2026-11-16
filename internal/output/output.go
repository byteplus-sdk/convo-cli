// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package output centralizes Agent-facing structured output.
//
// stdout carries data (a JSON envelope), stderr carries
// progress/warnings/hints. The two never interleave. `--format` selects the
// rendering of stdout: json (default, machine-first), pretty, or table.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"convo-cli/internal/errs"
)

// Format is the stdout rendering mode.
type Format string

const (
	FormatJSON   Format = "json"
	FormatPretty Format = "pretty"
	FormatTable  Format = "table"
)

// ParseFormat validates a --format value.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatJSON, FormatPretty, FormatTable:
		return Format(s), nil
	default:
		return "", errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation,
			"unknown format %q", s).WithParam("--format").
			WithHint("use one of: json, pretty, table")
	}
}

// Envelope is the top-level stdout payload for every command.
type Envelope struct {
	OK     bool           `json:"ok"`
	Data   any            `json:"data,omitempty"`
	Error  *errs.Error    `json:"error,omitempty"`
	Notice map[string]any `json:"_notice,omitempty"`
}

var noticeProvider struct {
	sync.RWMutex
	fn func() map[string]any
}

// SetNoticeProvider installs the cache-only notice composer used by JSON
// envelopes. Passing nil disables lifecycle notices.
func SetNoticeProvider(fn func() map[string]any) {
	noticeProvider.Lock()
	noticeProvider.fn = fn
	noticeProvider.Unlock()
}

func pendingNotice() map[string]any {
	noticeProvider.RLock()
	fn := noticeProvider.fn
	noticeProvider.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
}

// Prettier is implemented by data payloads that can render themselves for
// --format pretty/table. When absent, output falls back to indented JSON.
type Prettier interface {
	Pretty(w io.Writer)
}

// Writer emits envelopes honoring the selected format and stream separation.
type Writer struct {
	Out    io.Writer
	Err    io.Writer
	Format Format
}

// New returns a Writer bound to os.Stdout/os.Stderr.
func New(format Format) *Writer {
	return &Writer{Out: os.Stdout, Err: os.Stderr, Format: format}
}

// Progress writes a progress/status line to stderr (never stdout).
func (w *Writer) Progress(format string, a ...any) {
	fmt.Fprintln(w.Err, fmt.Sprintf(format, a...))
}

// Warn writes a warning to stderr.
func (w *Writer) Warn(format string, a ...any) {
	fmt.Fprintln(w.Err, "warn: "+fmt.Sprintf(format, a...))
}

// Data emits a successful envelope to stdout.
func (w *Writer) Data(data any) error {
	if w.Format == FormatJSON {
		return w.writeJSON(Envelope{OK: true, Data: data})
	}
	if p, ok := data.(Prettier); ok {
		var rendered bytes.Buffer
		p.Pretty(&rendered)
		_, err := io.WriteString(w.Out, rendered.String())
		return err
	}
	return w.writeJSON(Envelope{OK: true, Data: data})
}

// Fail emits an error envelope to stdout (so it stays parseable on the data
// channel) and mirrors JSON hints to stderr; returns the exit code.
func (w *Writer) Fail(err error) int {
	e, ok := errs.As(err)
	if !ok {
		e = errs.Wrap(err, "byteplus-rtc.cli.internal", "%s", err.Error())
	}
	// When the command already wrote a data envelope to stdout, don't emit a
	// second (error) envelope — just set the exit code and hint on stderr.
	if e.IsReported() {
		if e.Hint != "" {
			fmt.Fprintf(w.Err, "hint: %s\n", e.Hint)
		}
		return e.ExitCode()
	}
	if w.Format == FormatJSON {
		_ = w.writeJSON(Envelope{OK: false, Error: e})
		if e.Hint != "" {
			fmt.Fprintf(w.Err, "hint: %s\n", e.Hint)
		}
	} else {
		fmt.Fprintf(w.Out, "%s\n", renderErrorHuman(e))
	}
	return e.ExitCode()
}

func (w *Writer) writeJSON(env Envelope) error {
	env.Notice = pendingNotice()
	var rendered bytes.Buffer
	enc := json.NewEncoder(&rendered)
	enc.SetIndent("", "  ")
	if err := enc.Encode(env); err != nil {
		return err
	}
	_, err := io.WriteString(w.Out, rendered.String())
	return err
}

func renderErrorHuman(e *errs.Error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✗ [%s] %s", e.Code, e.Message)
	if e.Param != "" {
		fmt.Fprintf(&b, "\n  param: %s", e.Param)
	}
	if e.Hint != "" {
		fmt.Fprintf(&b, "\n  hint:  %s", e.Hint)
	}
	return b.String()
}
