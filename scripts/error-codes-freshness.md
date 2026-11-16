# Error-code knowledge-base freshness

`internal/errorcodes/*.yaml` provides the offline knowledge base for
`explain-error`. Review the source documents before a release and update the
local entries when their documented behavior changes.

## Sources

Ordinary builds load `byteplus-web.yaml` and `byteplus-voice-agent.yaml`.
Reference builds load `web.yaml` and `voice-agent.yaml`. Each entry records its
source and verification state; source groups may also record `source_version`.
Use the source metadata in the corresponding YAML file to locate the document.

## Review process

1. Read each source document and compare it with the local entries and recorded
   source version or verification date.
2. Update changed meanings, recovery guidance, and newly documented codes.
3. Update `source_version` and `verified_at` for the entries reviewed.
4. Set `verified: verified` when the source confirms the entry. Use
   `verified: curated-seed` for general troubleshooting guidance and
   `verified: unverified` when verification remains pending.
5. Run `make test`. Follow `AGENTS.md` when changing CLI error codes or snapshots.

Write maintained guidance in English. Preserve official identifiers, links, and
source provenance. Consult the current official documentation for VoiceChat
request parameters and other API contracts.

## Verification states

- `verified`: checked against the cited official public documentation.
- `curated-seed`: general troubleshooting guidance awaiting per-entry verification.
- `unverified`: source verification is pending.

`explain-error` includes a verification warning for entries whose state differs
from `verified`.
