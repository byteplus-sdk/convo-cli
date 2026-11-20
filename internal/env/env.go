// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package env reads/merges/writes a local dotenv file for a project. `env write`
// collects credentials and base config without echoing secret values.
package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
)

// FileName is the local env file written by `env write`.
const FileName = ".env.local"

// sensitiveKeys are never echoed back in full.
var sensitiveKeys = map[string]bool{
	"RTC_APP_KEY":    true,
	"VITE_RTC_TOKEN": true,
	"RTC_TOKEN":      true,
}

// IsSensitive reports whether a key holds a secret.
func IsSensitive(key string) bool {
	if sensitiveKeys[strings.ToUpper(key)] {
		return true
	}
	up := strings.ToUpper(key)
	return strings.Contains(up, "KEY") || strings.Contains(up, "SECRET") || strings.Contains(up, "TOKEN")
}

// Mask redacts a secret value for display.
func Mask(val string) string {
	if val == "" {
		return ""
	}
	if len(val) <= 4 {
		return "****"
	}
	return val[:2] + strings.Repeat("*", len(val)-4) + val[len(val)-2:]
}

// Path returns the env file path for a project dir.
func Path(dir string) string { return filepath.Join(dir, FileName) }

// LoadIntoProcess loads the nearest .env.local (walking up from dir) into the
// process environment, WITHOUT overriding variables already set (the real
// environment wins, standard dotenv precedence). It makes credentials written by
// `env write` / interactive dev setup (e.g. RTC_APP_ID / RTC_APP_KEY) visible to the
// CLI's own `${ENV}` resolution — the same file the web runtime (Vite) reads — so
// `token issue` / `doctor` / `dev` work without a manual `export`. Returns the
// keys it set. A missing file is not an error.
func LoadIntoProcess(dir string) ([]string, error) {
	path, ok := findUp(dir, FileName)
	if !ok {
		return nil, nil
	}
	values, err := Load(path)
	if err != nil {
		return nil, err
	}
	var set []string
	for k, v := range values {
		if _, exists := os.LookupEnv(k); exists {
			continue // real environment wins
		}
		if err := os.Setenv(k, v); err != nil {
			return set, errs.Wrap(err, "byteplus-rtc.cli.internal", "set env %s: %s", k, err)
		}
		set = append(set, k)
	}
	return set, nil
}

// findUp walks up from dir looking for name, returning its path when found.
func findUp(dir, name string) (string, bool) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		p := filepath.Join(abs, name)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

// Load parses a dotenv file into a map (missing file → empty map).
func Load(path string) (map[string]string, error) {
	m := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, errs.Wrap(err, "byteplus-rtc.cli.internal", "read env file: %s", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		m[key] = value
	}
	return m, sc.Err()
}

var dotenvKey = regexp.MustCompile(`^[\w.-]+$`)
var quotedDotenvValue = regexp.MustCompile(`^\s*('(?:\\'|[^'])*'|"(?:\\"|[^"])*"|\x60(?:\\\x60|[^\x60])*\x60)([ \t]*(?:#.*)?)$`)

func parseLine(raw string) (key, value string, ok bool) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	if rest := strings.TrimPrefix(line, "export "); rest != line {
		line = strings.TrimSpace(rest)
	}
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	if !dotenvKey.MatchString(k) {
		return "", "", false
	}
	value, _ = splitValue(v)
	return k, value, true
}

func splitValue(raw string) (value, suffix string) {
	if parts := quotedDotenvValue.FindStringSubmatch(raw); parts != nil {
		quoted := parts[1]
		value = quoted[1 : len(quoted)-1]
		if quoted[0] == '"' {
			value = strings.NewReplacer(`\n`, "\n", `\r`, "\r").Replace(value)
		}
		return value, parts[2]
	}
	before, _, hasComment := strings.Cut(raw, "#")
	value = strings.TrimRight(before, " \t")
	if hasComment {
		suffix = raw[len(value):]
	}
	return strings.TrimSpace(value), suffix
}

func encodeValue(key, value string) (string, error) {
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", errs.New("byteplus-rtc.env.write_failed", errs.TypeValidation, "environment variable %q requires a single-line value", key)
	}
	if value == "" || strings.TrimSpace(value) == value && !strings.Contains(value, "#") && !strings.ContainsRune("'\"`", rune(value[0])) {
		return value, nil
	}
	for _, quote := range []string{"'", "`", "\""} {
		if strings.Contains(value, quote) {
			continue
		}
		encoded := quote + value + quote
		if decoded, _ := splitValue(encoded); decoded == value {
			return encoded, nil
		}
	}
	return "", errs.New("byteplus-rtc.env.write_failed", errs.TypeValidation, "environment variable %q cannot be represented as a dotenv value", key)
}

// Write merges updates into the env file at path, preserving existing keys.
// Returns the set of keys that were written (for a masked summary).
func Write(path string, updates map[string]string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, errs.Wrap(err, "byteplus-rtc.env.write_failed", "read local environment")
	}
	example, err := os.ReadFile(filepath.Join(filepath.Dir(path), ".env.example"))
	if err != nil && !os.IsNotExist(err) {
		return nil, errs.Wrap(err, "byteplus-rtc.env.write_failed", "read environment template")
	}
	if len(raw) == 0 {
		raw = example
		if len(raw) == 0 {
			raw = []byte(fmt.Sprintf("# Written by `%s env write`. Secrets are env-only; do not commit.\n", meta.BinName))
		}
	}
	newline := "\n"
	if strings.Contains(string(raw), "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n"), "\n")
	if len(example) > 0 && string(raw) != string(example) {
		lines = restoreTemplateLayout(lines, string(example))
	}
	present := map[string]bool{}
	for _, line := range lines {
		if key, _, ok := parseLine(line); ok {
			present[key] = true
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(example), "\r\n", "\n"), "\n") {
		if key, _, ok := parseLine(line); ok && !present[key] {
			lines = append(lines, line)
			present[key] = true
		}
	}
	encoded := map[string]string{}
	var written []string
	for k := range updates {
		if !dotenvKey.MatchString(k) {
			return nil, errs.New("byteplus-rtc.env.write_failed", errs.TypeValidation, "invalid environment variable name")
		}
		value, err := encodeValue(k, updates[k])
		if err != nil {
			return nil, err
		}
		encoded[k] = value
		written = append(written, k)
	}
	sort.Strings(written)
	for i, line := range lines {
		key, previous, ok := parseLine(line)
		if value, update := encoded[key]; ok && update && previous != updates[key] {
			left, right, _ := strings.Cut(line, "=")
			_, suffix := splitValue(right)
			lines[i] = left + "=" + value + suffix
		}
	}
	for _, key := range written {
		if !present[key] {
			lines = append(lines, key+"="+encoded[key])
		}
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, errs.New("byteplus-rtc.env.write_failed", errs.TypeIO, "create dir: %s", err)
		}
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".env-*")
	if err != nil {
		return nil, errs.New("byteplus-rtc.env.write_failed", errs.TypeIO, "create env file: %s", err)
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(strings.Join(lines, newline) + newline); err != nil {
		_ = temp.Close()
		return nil, errs.New("byteplus-rtc.env.write_failed", errs.TypeIO, "write %q: %s", path, err)
	}
	if err := temp.Close(); err != nil {
		return nil, errs.New("byteplus-rtc.env.write_failed", errs.TypeIO, "close %q: %s", path, err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return nil, errs.New("byteplus-rtc.env.write_failed", errs.TypeIO, "write %q: %s", path, err)
	}
	return written, nil
}

func restoreTemplateLayout(lines []string, example string) []string {
	if !sort.StringsAreSorted(lines) {
		return lines
	}
	local := make(map[string]string, len(lines))
	for _, line := range lines {
		key, value, ok := parseLine(line)
		if !ok || line != key+"="+value || strings.Contains(line, "#") {
			return lines
		}
		if _, duplicate := local[key]; duplicate {
			return lines
		}
		local[key] = line
	}
	layout := strings.Split(strings.TrimSuffix(strings.ReplaceAll(example, "\r\n", "\n"), "\n"), "\n")
	missing := false
	used := make(map[string]bool)
	for i, line := range layout {
		key, _, ok := parseLine(line)
		if !ok {
			continue
		}
		if original, found := local[key]; found {
			layout[i] = original
			used[key] = true
		} else {
			missing = true
		}
	}
	if !missing {
		return lines
	}
	for _, line := range lines {
		key, _, _ := parseLine(line)
		if !used[key] {
			layout = append(layout, line)
		}
	}
	return layout
}
