// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"convo-cli/internal/errs"
	"convo-cli/internal/product"
)

func newTestWriter(f Format) (*Writer, *bytes.Buffer, *bytes.Buffer) {
	var o, e bytes.Buffer
	return &Writer{Out: &o, Err: &e, Format: f}, &o, &e
}

func TestDataJSONEnvelope(t *testing.T) {
	SetNoticeProvider(nil)
	w, o, e := newTestWriter(FormatJSON)
	w.Progress("progress goes to stderr")
	if err := w.Data(map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, o.String())
	}
	if !env.OK {
		t.Fatal("expected ok=true")
	}
	// stdout must not contain the progress line.
	if strings.Contains(o.String(), "progress goes to stderr") {
		t.Fatal("progress leaked into stdout")
	}
	if !strings.Contains(e.String(), "progress goes to stderr") {
		t.Fatal("progress missing from stderr")
	}
}

func TestJSONEnvelopeNotice(t *testing.T) {
	SetNoticeProvider(func() map[string]any {
		return map[string]any{"update": map[string]string{"latest": "1.2.3"}}
	})
	t.Cleanup(func() { SetNoticeProvider(nil) })
	w, o, _ := newTestWriter(FormatJSON)
	if err := w.Data(map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Notice["update"] == nil {
		t.Fatalf("missing update notice: %+v", env)
	}
}

func TestFailJSONEnvelopeNotice(t *testing.T) {
	SetNoticeProvider(func() map[string]any { return map[string]any{"skills": "stale"} })
	t.Cleanup(func() { SetNoticeProvider(nil) })
	w, o, _ := newTestWriter(FormatJSON)
	w.Fail(errs.New("byteplus-rtc.cli.internal", errs.TypeInternal, "boom"))
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Notice["skills"] == nil || env.Error == nil {
		t.Fatalf("unexpected envelope: %+v", env)
	}
}

func TestFailEnvelopeAndExit(t *testing.T) {
	w, o, _ := newTestWriter(FormatJSON)
	code := w.Fail(errs.New("byteplus-rtc.config.missing_field", errs.TypeValidation, "boom").
		WithParam("rtc.app_id").WithHint("fix it"))
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.OK || env.Error == nil || env.Error.Code != "byteplus-rtc.config.missing_field" {
		t.Fatalf("unexpected error envelope: %+v", env)
	}
}

func TestFailEnvelopePreservesRegisteredCode(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })

	internal := errs.New("byteplus-rtc.config.not_found", errs.TypeNotFound, "config missing")
	w, o, _ := newTestWriter(FormatJSON)
	w.Fail(internal)
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Code != "byteplus-rtc.config.not_found" {
		t.Fatalf("BytePlus public error = %+v", env.Error)
	}
	if internal.Code != "byteplus-rtc.config.not_found" {
		t.Fatalf("internal catalog code mutated to %q", internal.Code)
	}
}

func TestDataAndProgressPreserveErrorCodes(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })

	w, o, e := newTestWriter(FormatJSON)
	w.Progress("retry after %s", "byteplus-rtc.auth.token_exchange_failed")
	if err := w.Data(map[string]string{"detail": "byteplus-rtc.openapi.request_failed: unavailable"}); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"stdout": o.String(), "stderr": e.String()} {
		if !strings.Contains(value, "byteplus-rtc.") {
			t.Fatalf("%s leaked internal prefix: %q", name, value)
		}
	}
}

func TestFailEnvelopeIncludesStructuredDetails(t *testing.T) {
	w, o, _ := newTestWriter(FormatJSON)
	w.Fail(errs.New("byteplus-rtc.dev.selection_required", errs.TypePrecondition, "choose").
		WithDetails(map[string]any{"apps": []map[string]string{{"app_id": "public"}}}))
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Details["apps"] == nil {
		t.Fatalf("missing details: %+v", env)
	}
}

func TestFailReportedSkipsStdout(t *testing.T) {
	w, o, e := newTestWriter(FormatJSON)
	err := errs.New("byteplus-rtc.doctor.failed", errs.TypePrecondition, "1 failed").
		WithHint("run doctor").Reported()
	code := w.Fail(err)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if o.Len() != 0 {
		t.Fatalf("reported error should not write stdout, got %q", o.String())
	}
	if !strings.Contains(e.String(), "run doctor") {
		t.Fatal("hint should still go to stderr")
	}
}

func TestFailPrettyRendersHintOnce(t *testing.T) {
	w, o, e := newTestWriter(FormatPretty)
	w.Fail(errs.New("byteplus-rtc.skills.sync_failed", errs.TypeIO, "failed").WithHint("retry sync"))
	if count := strings.Count(o.String()+e.String(), "retry sync"); count != 1 {
		t.Fatalf("hint count=%d, stdout=%q stderr=%q", count, o.String(), e.String())
	}
}

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"json", "pretty", "table"} {
		if _, err := ParseFormat(ok); err != nil {
			t.Fatalf("ParseFormat(%q): %v", ok, err)
		}
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Fatal("expected error for xml")
	}
}

func TestBytePlusDataPreservesPathsAndURLs(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	payload := map[string]string{
		"path":  "/project/app.config.yaml",
		"url":   "https://app.example.com/docs",
		"value": "arbitrary.custom.value",
	}
	w, stdout, _ := newTestWriter(FormatJSON)
	if err := w.Data(payload); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for key, want := range payload {
		if got := envelope.Data[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type prettyPayload struct{}

func (prettyPayload) Pretty(w io.Writer) { _, _ = io.WriteString(w, "value") }
func TestPrettyDataPropagatesWriteError(t *testing.T) {
	writer := &Writer{Out: failingWriter{}, Format: FormatPretty}
	if err := writer.Data(prettyPayload{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Data error = %v, want closed pipe", err)
	}
}
