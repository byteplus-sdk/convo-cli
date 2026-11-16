<a id="english"></a>

# Automation and structured output

`convo-cli` uses readable output when stdout is a terminal and JSON when stdout is
piped or redirected. Coding agents, scripts, and CI should pass `--format json`
explicitly so their output does not depend on the terminal environment.

## Stream contract

- stdout contains command data only.
- stderr contains progress, warnings, and recovery hints.
- in JSON mode, a command writes one envelope to stdout.
- success exits with code 0; failure exits non-zero.

A successful result has this shape:

```json
{
  "ok": true,
  "data": {}
}
```

A known failure has this shape:

```json
{
  "ok": false,
  "error": {
    "code": "byteplus-rtc.example.code",
    "type": "validation",
    "message": "...",
    "hint": "..."
  }
}
```

Fields such as `subtype` and `param` are included when relevant. Automation
should branch on `ok`, process exit status, and `error.code` before interpreting
human-language messages.

## Output formats

```bash
convo-cli doctor                 # pretty in a terminal, JSON when redirected
convo-cli doctor --format json   # stable choice for automation
convo-cli doctor --format pretty
convo-cli init --list --format table
```

Progress on stderr never changes the stdout representation. Redirect streams
separately when a pipeline needs both structured results and diagnostic logs.

## Dry runs

The global `--dry-run` flag previews side effects without applying them:

```bash
convo-cli init --scene voice-agent --platform web --dry-run
convo-cli dev --dry-run
convo-cli update --dry-run
```

For remote templates, a dry run may validate from memory or an existing cache,
but it does not create the target or populate the cache.
An update dry run may query the npm registry and detect the install method, but
it never invokes npm, replaces a binary, writes lifecycle state, or synchronizes
Skills. Its `status` is `would_update`, `up_to_date`, or `manual_required`.

## Authentication in automation

Authorization Code with PKCE requires a user to approve access. On a headless
host, use:

```bash
convo-cli auth login --browser=manual --store=file
```

The command prints the authorization URL and accepts the returned code through
stdin. Signin tokens default to the protected `$CONVO_CLI_HOME/auth.json` file,
outside the project; use `--store=keyring` only after explicit user consent.
`--browser=ask` returns `byteplus-rtc.auth.interaction_required` in non-interactive
terminals instead of opening UI. After login, run `dev` directly. If a user decision is required,
it returns a typed JSON error containing public candidates:

```bash
convo-cli dev
# on byteplus-rtc.dev.selection_required, choose from error.details
convo-cli dev --app-id <app-id>
```

A unique RTC application is selected automatically. When several applications
are available, select a public AppID from `error.details.apps` and retry with
`--app-id`. The CLI retrieves AppKey with Signin STS and keeps it local.
Configure model and speech resources with `convo-cli agent resources --write` and
`server/scenes/default.json`; see [Voice-agent projects](./voice-agent.md#english).
Automation must keep AppKey out of chat, command arguments, logs, and `VITE_*`.

For an environment whose project credentials and scene configuration are
already present, run `convo-cli doctor` to check configuration before `convo-cli dev`.

Resource selection errors include candidates in `error.details.choices` and retry
arguments in `error.details.retry_argv`. Replace the placeholders with chosen IDs;
the arguments preserve existing selections.

For web-server projects, wait for `data.status: ready` before using `data.urls`.
Stdout contains one JSON result; progress and service logs go to stderr. Continue
watching the process exit code: a later service failure returns a nonzero code.

## Lifecycle notices

JSON envelopes may contain additive `_notice.update` and `_notice.skills`
objects. They are composed from local cached state and do not add synchronous
network I/O to the output path. Consumers must tolerate unknown additive notice
fields.

Agents must finish the current user task before surfacing a lifecycle notice and
must not update autonomously. `_notice.update` maps to `convo-cli update`;
`_notice.skills` maps to `convo-cli skills sync`.

Suppress them when necessary:

```bash
export CONVO_CLI_NO_UPDATE_NOTIFIER=1
export CONVO_CLI_NO_SKILLS_NOTIFIER=1
```

## Official Skills

Synchronize the official scene workflow embedded in the installed CLI release:

```bash
convo-cli skills sync
```

Synchronization copies the embedded content into supported global Agent
runtimes and requires `npx skills` to be available. To preview without writing,
run `convo-cli skills sync --dry-run`.

The CLI also embeds the Skill content shipped with its release:

```bash
convo-cli skills list
convo-cli skills read byted-bp-convo-guide
convo-cli skills sync
```

For every public release and snapshot artifact, the value printed
by `convo-cli version`, the `version` frontmatter returned by
`convo-cli skills read <name>`, and the corresponding archived `SKILL.md` version
are identical. Release postflight reads the embedded Markdown itself; consumers
therefore do not need repository access or a separate Skill-version mapping.

`convo-cli update` reuses the same synchronization after updating an npm-managed
binary. `convo-cli skills sync` keeps the installed content tied to the running
binary release.

## Failure routing

1. Inspect the non-zero exit status and JSON `error.code`.
2. Follow the structured `hint` when present.
3. Run `convo-cli doctor` for readiness or configuration failures.
4. Run `convo-cli explain-error <code>` for Web SDK or conversational-AI runtime
   codes. These runtime codes are separate from CLI `error.code` values.

---
