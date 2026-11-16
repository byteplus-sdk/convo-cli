// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"regexp"
	"sort"
	"strings"
)

// CatalogEntry documents one stable error.code.
type CatalogEntry struct {
	Code    string `json:"code"`
	Type    Type   `json:"type"`
	Summary string `json:"summary"`
}

// codePattern enforces the `byteplus-rtc.<domain>.<subtype>` shape. The CI check
// (scripts/snapshot-error-codes) asserts every code emitted by the binary is
// registered here and matches this pattern.
var codePattern = regexp.MustCompile(`^byteplus-rtc\.[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)

// Catalog is the converged registry of every CLI-owned error.code.
// Adding or renaming a code REQUIRES a matching entry here; the CI snapshot
// check fails otherwise.
var Catalog = []CatalogEntry{
	// --- cli / foundation ---
	{"byteplus-rtc.cli.unknown_command", TypeNotFound, "Subcommand does not exist; hint suggests nearest candidate"},
	{"byteplus-rtc.cli.invalid_flag", TypeValidation, "A flag received an invalid value"},
	{"byteplus-rtc.cli.internal", TypeInternal, "Unexpected internal CLI failure"},
	{"byteplus-rtc.cli.not_implemented", TypePrecondition, "Feature is not available"},

	// --- update lifecycle ---
	{"byteplus-rtc.update.network", TypeIO, "Failed to query npm for the latest CLI release"},
	{"byteplus-rtc.update.detect_failed", TypeIO, "Failed to detect how the running CLI was installed"},
	{"byteplus-rtc.update.manual_install", TypePrecondition, "Self-update refused to overwrite a manually installed binary"},
	{"byteplus-rtc.update.failed", TypeIO, "npm failed to install the requested CLI release"},
	{"byteplus-rtc.update.verify_failed", TypeIO, "The updated CLI binary did not report the expected version"},

	// --- auth ---
	{"byteplus-rtc.auth.not_authenticated", TypeAuth, "Command requires authentication; run auth login"},
	{"byteplus-rtc.auth.authorization_failed", TypeAuth, "Signin authorization failed or callback was invalid"},
	{"byteplus-rtc.auth.interaction_required", TypePrecondition, "Signin requires an explicit browser choice in a non-interactive terminal"},
	{"byteplus-rtc.auth.invalid_callback", TypeValidation, "OAuth loopback callback configuration is invalid"},
	{"byteplus-rtc.auth.invalid_token", TypeAuth, "Stored auth token is missing or cannot be used"},
	{"byteplus-rtc.auth.storage_unavailable", TypeAuth, "Selected credential storage is unavailable or unsafe"},
	{"byteplus-rtc.auth.token_exchange_failed", TypeAuth, "OAuth token endpoint exchange or refresh failed"},

	// --- config ---
	{"byteplus-rtc.config.not_found", TypeNotFound, "byteplus-rtc.config.yaml not found in the current project"},
	{"byteplus-rtc.config.parse_error", TypeValidation, "byteplus-rtc.config.yaml could not be parsed"},
	{"byteplus-rtc.config.missing_field", TypeValidation, "A required config field is missing"},
	{"byteplus-rtc.config.invalid_field", TypeValidation, "A config field has an invalid type or value"},
	{"byteplus-rtc.config.unresolved_env", TypeValidation, "A ${ENV} placeholder cannot be resolved"},
	{"byteplus-rtc.config.write_failed", TypeIO, "Failed to persist byteplus-rtc.config.yaml"},
	{"byteplus-rtc.config.invalid_identity", TypeValidation, "room_id/user_id is empty, too long, or has disallowed characters"},

	// --- token ---
	{"byteplus-rtc.token.missing_credential", TypeAuth, "AppKey (or other credential) is not available"},
	{"byteplus-rtc.token.missing_config", TypeValidation, "app_id/room_id/user_id missing for token issue"},
	{"byteplus-rtc.token.invalid_appid", TypeValidation, "AppID is not the required 24-character length"},
	{"byteplus-rtc.token.invalid", TypeValidation, "Token failed to parse or verify"},
	{"byteplus-rtc.token.expired", TypeValidation, "Token is expired"},

	// --- scaffolding / template ---
	{"byteplus-rtc.template.not_found", TypeNotFound, "No template for the requested scene x platform"},
	{"byteplus-rtc.template.render_failed", TypeIO, "Template rendering failed"},
	{"byteplus-rtc.template.download_failed", TypeIO, "Remote template archive could not be downloaded or cached"},
	{"byteplus-rtc.template.checksum_mismatch", TypeValidation, "Remote template archive did not match its pinned SHA-256"},
	{"byteplus-rtc.template.contract_invalid", TypeValidation, "Remote template archive or manifest violates the template contract"},
	{"byteplus-rtc.init.target_exists", TypePrecondition, "Target directory already exists and is not empty"},

	// --- env ---
	{"byteplus-rtc.env.write_failed", TypeIO, "Failed to write environment file"},

	// --- doctor ---
	{"byteplus-rtc.doctor.failed", TypePrecondition, "One or more doctor checks reported FAIL"},

	// --- explain-error ---
	{"byteplus-rtc.explain.unknown_code", TypeNotFound, "Error code is not in the offline knowledge base"},

	// --- OpenAPI ---
	{"byteplus-rtc.openapi.http_error", TypePrecondition, "OpenAPI endpoint returned a non-success HTTP status"},
	{"byteplus-rtc.openapi.invalid_request", TypeValidation, "OpenAPI request flags or payload are invalid"},
	{"byteplus-rtc.openapi.request_failed", TypeIO, "OpenAPI transport/HTTP call failed"},
	{"byteplus-rtc.openapi.sign_failed", TypeValidation, "OpenAPI request signing failed"},

	// --- Topic RTC documentation MCP ---
	{"byteplus-rtc.docs.invalid_argument", TypeValidation, "Documentation command argument or local paging value is invalid"},
	{"byteplus-rtc.docs.request_failed", TypeIO, "RTC documentation transport request failed"},
	{"byteplus-rtc.docs.timeout", TypeIO, "RTC documentation lifecycle or tool call timed out"},
	{"byteplus-rtc.docs.http_error", TypePrecondition, "RTC documentation service returned an unexpected HTTP status"},
	{"byteplus-rtc.docs.rate_limited", TypePrecondition, "RTC documentation service rate limit was exceeded"},
	{"byteplus-rtc.docs.unauthorized", TypeAuth, "RTC documentation service rejected authorization"},
	{"byteplus-rtc.docs.unsupported_protocol", TypePrecondition, "RTC documentation MCP negotiated an unsupported protocol version"},
	{"byteplus-rtc.docs.tool_unavailable", TypePrecondition, "Required RTC documentation tool is unavailable"},
	{"byteplus-rtc.docs.tool_schema_changed", TypePrecondition, "RTC documentation tool schema is incompatible"},
	{"byteplus-rtc.docs.protocol_error", TypeIO, "RTC documentation MCP response or nested payload violates the expected contract"},
	{"byteplus-rtc.docs.tool_error", TypePrecondition, "RTC documentation tool returned an application error"},
	{"byteplus-rtc.docs.document_not_found", TypeNotFound, "Requested RTC documentation document was not found"},
	{"byteplus-rtc.docs.response_too_large", TypePrecondition, "Decoded RTC documentation response exceeded the safety limit"},

	// --- dev ---
	{"byteplus-rtc.dev.config_incomplete", TypePrecondition, "Config incomplete; run doctor before dev"},
	{"byteplus-rtc.dev.app_setup_failed", TypePrecondition, "RTC application discovery or credential setup failed"},
	{"byteplus-rtc.dev.port_in_use", TypePrecondition, "A template development port is already occupied"},
	{"byteplus-rtc.dev.task_failed", TypeIO, "Template dev task exited non-zero"},

	// --- conversational agent (StartVoiceChat / StopVoiceChat) ---
	{"byteplus-rtc.agent.missing_config", TypeValidation, "Required agent (asr/tts/llm) config is missing"},
	{"byteplus-rtc.agent.no_task", TypePrecondition, "No running agent task for stop/update/status"},
	{"byteplus-rtc.agent.server_managed", TypePrecondition, "Agent lifecycle is owned by the generated companion server"},

	{"byteplus-rtc.openapi.api_error", TypePrecondition, "RTC OpenAPI returned an error code"},

	// --- embedded skills (skills list/read) ---
	{"byteplus-rtc.skills.unavailable", TypeInternal, "Skill content is not embedded in this build"},
	{"byteplus-rtc.skills.unknown_skill", TypeNotFound, "Requested skill is not an official embedded skill"},
	{"byteplus-rtc.skills.invalid_path", TypeValidation, "Skill reference path is absolute or escapes the skill root"},
	{"byteplus-rtc.skills.not_found", TypeNotFound, "Requested skill reference does not exist"},
	{"byteplus-rtc.skills.sync_failed", TypeIO, "Official embedded Skills could not be synchronized to Agent runtimes"},

	// --- interactive dev setup ---
	{"byteplus-rtc.dev.interactive_required", TypePrecondition, "Deprecated: interactive dev setup requires a public resource selection; use byteplus-rtc.dev.selection_required"},
	{"byteplus-rtc.dev.selection_required", TypePrecondition, "Dev setup requires a public resource selection"},
	{"byteplus-rtc.provision.no_app", TypeNotFound, "No active RTC app is available for interactive setup"},
	{"byteplus-rtc.provision.no_bot", TypeNotFound, "No usable conversational-AI bot is available for scene setup"},
	{"byteplus-rtc.provision.not_selected", TypeValidation, "Interactive bot selection was not completed"},
	{"byteplus-rtc.provision.write_failed", TypeIO, "Failed to validate or write a generated bot scene"},
}

var catalogIndex = func() map[string]CatalogEntry {
	m := make(map[string]CatalogEntry, len(Catalog))
	for _, e := range Catalog {
		m[e.Code] = e
	}
	return m
}()

// IsRegistered reports whether code exists in the catalog.
func IsRegistered(code string) bool {
	_, ok := catalogIndex[code]
	return ok
}

// ValidateCatalog checks every entry matches the code pattern and is unique.
// Returns a sorted list of problems (empty when healthy). Used by the CI check.
func ValidateCatalog() []string {
	var problems []string
	seen := map[string]bool{}
	for _, e := range Catalog {
		if !codePattern.MatchString(e.Code) {
			problems = append(problems, "invalid code shape: "+e.Code)
		}
		if seen[e.Code] {
			problems = append(problems, "duplicate code: "+e.Code)
		}
		seen[e.Code] = true
		if strings.TrimSpace(e.Summary) == "" {
			problems = append(problems, "missing summary: "+e.Code)
		}
	}
	sort.Strings(problems)
	return problems
}

func subtypeOf(code string) string {
	parts := strings.Split(code, ".")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return ""
}
