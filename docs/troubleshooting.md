<a id="english"></a>

# Troubleshooting

Start with the diagnostic command that matches the failure:

```bash
convo-cli doctor --format pretty          # CLI and current project
convo-cli doctor cli --format pretty      # installation only
convo-cli doctor project --format pretty  # current project only
```

`doctor` is read-only. Every failed check includes a recovery hint and a JSON
check identifier when run in the default format.

## `convo-cli` is not found

Confirm Node.js 16 or later is installed, reinstall the package, and check the
npm global binary directory on `PATH`:

```bash
npm install -g @byteplus/convo-cli
convo-cli version --format pretty
```

If the package install completed but its binary download failed, verify access
to GitHub Releases. A source build with Go 1.26.11 or later is an alternative:

```bash
cd convo-cli
make build
./bin/convo-cli version --format pretty
```

## Template download fails

The first `voice-agent` scaffold downloads a pinned archive from GitHub
codeload. Check network or proxy access and retry the same command. A fully
verified earlier download is cached and can be reused offline.

`--dry-run` does not populate an empty cache. It can only validate from memory
during a successful network request or reuse an existing cached archive.

## Browser login does not open

Run browser authorization:

```bash
convo-cli auth login --browser=open --format pretty
```

If opening the browser fails, open the printed URL and complete authorization while the command waits for the local callback. Login establishes Signin; project application configuration happens on the next `convo-cli dev`.
In a non-interactive terminal, the default `--browser=ask` returns
`byteplus-rtc.auth.interaction_required` instead of opening UI; choose `open` only
after user consent. Credentials use protected file storage by default, or the
OS credential store after explicit `--store=keyring` selection.
In a non-interactive terminal, run `convo-cli dev`. If it returns
`byteplus-rtc.dev.selection_required`, select a public AppID from
`error.details.apps` and retry with `--app-id <app-id>`. Signin remains valid
when project configuration fails. Retry `convo-cli dev --reconfigure` or follow
[First-run configuration](./voice-agent.md#first-run-configuration-from-the-console).

## Changing accounts

Stop the running dev process with Ctrl+C. In the project directory, authorize the intended account and choose its model and speech resources:

```bash
convo-cli auth login --force --browser=open
convo-cli agent resources --write
convo-cli dev --reconfigure
```

Once the services are running, use `convo-cli agent check` from another terminal in the project directory to check provider configuration and speech access. The Demo also checks its provider selection before starting a conversation.

Select the intended account in the browser. `dev --reconfigure` selects an RTC application and refreshes its AppKey. Resource selection clears the saved model API key and speech access token in `.env.local`; the CLI obtains credentials for the selected resources when needed. A selected model API key ID takes precedence over `BYTEPLUS_MODEL_API_KEY`.

Review `BYTEPLUS_ACCESS_KEY_ID` and `BYTEPLUS_SECRET_ACCESS_KEY` in the shell and `.env.local`: the companion server uses these credentials when configured, and CLI requests can use them as an authentication fallback. Update or unset them when changing accounts. Update or unset exported RTC and provider variables as well; shell variables take precedence over `.env.local`. Restart the dev process after saving configuration.

For `AccessDenied.NoPermissionForApp`, choose an RTC AppID owned by or shared with the authenticated account. Ask the application's owner or your organization's BytePlus account administrator to grant access. For Seed Speech authorization failures, ask that administrator to grant application access and activate ASR and TTS. They can confirm the owning account and arrange access; the CLI cannot determine your organization's administrator.

## No RTC application is available

Open the BytePlus Console and confirm that the signed-in identity can access an
RTC application and its AppKey. IAM subaccounts need `DescribeApps` and `DescribeAppKeys` permission.
If the Project selector is empty, ask the account administrator to check project visibility and project permissions. After an application becomes available, retry `convo-cli dev --reconfigure`.
Use `convo-cli agent resources --write` to select model and speech resources. See [Voice-agent projects](./voice-agent.md#english).

## `RTC_APP_KEY is not set`

Run `convo-cli auth login`, then run `convo-cli dev`; in a non-interactive terminal,
choose only public AppIDs from a `byteplus-rtc.dev.selection_required` error's
`details`. The CLI writes AppId/AppKey to `.env.local` without exposing AppKey in
process arguments. If a manual fallback is required, use the
local editor or secret-management workflow described in
[First-run configuration from the Console](./voice-agent.md#first-run-configuration-from-the-console).
Never pass AppKey as a command-line argument or put it in `byteplus-rtc.config.yaml`, a
`VITE_*` variable, client-side code, chat, logs, or an issue report. Confirm
`.env.local` remains ignored by Git.

## Agent and browser are not interacting

Run `convo-cli doctor project --format pretty` and check room/user consistency. In
CLI-managed workflows, `rtc.room_id`, `rtc.user_id`, and the agent target must
refer to the same conversation. See
[Room and user identity](./voice-agent.md#room-and-user-identity).

## A runtime error code appears

Query the embedded offline knowledge base:

```bash
convo-cli explain-error <code> --format pretty
```

The result includes the known meaning, recommended fix, and related doctor
check when available.

## Still blocked

Read [SUPPORT.md](../SUPPORT.md#english) before opening an issue. Include the CLI
version, operating system, command, exit code, redacted JSON error envelope, and
relevant `doctor` checks. Never include credentials, tokens, `.env.local`, or
private Console data.

---
