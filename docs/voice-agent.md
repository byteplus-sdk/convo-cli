<a id="english"></a>

# Voice-agent projects

`convo-cli init` scaffolds the commit-pinned, SHA-256 verified `byteplus-sdk/RTC_AIGC_Demo` with `@byteplus/rtc`. Cached archives support creating the same template version offline.

## First-run configuration from the Console

```bash
convo-cli init ./my-agent --scene voice-agent --platform web
cd ./my-agent
convo-cli auth login --browser=open
convo-cli agent resources --write
convo-cli dev
```

Run `convo-cli agent resources --write` to save the RTC application and provider settings in `.env.local`. Use `--app-id <id>` for a specific application. Application discovery uses DescribeApps and key retrieval uses DescribeAppKeys. The browser Demo can issue tokens through GetToken when AppKey access is denied. Run `convo-cli dev` to start the configured project.

`agent resources --write` lists model and speech resources, prompts for selection, and saves the selected IDs in `.env.local`. Review `VoiceChat.Config` and `VoiceChat.AgentConfig` in `server/scenes/default.json`. Provider credentials use local environment references. Scene configuration supports `${ENV}` and reports unresolved values as typed errors.

Signin state defaults to `~/.convo-cli/auth.json`; `CONVO_CLI_HOME` overrides its directory. Use `--store=keyring` for the operating-system credential store. Server-side credentials support `BYTEPLUS_ACCESS_KEY_ID`, `BYTEPLUS_SECRET_ACCESS_KEY` and optional `BYTEPLUS_SESSION_TOKEN`.

Run `convo-cli auth login --project-auth` to use the project’s `.convo-cli/` directory. The command saves `auth.project: true` in the project configuration and adds a Git ignore rule. Later commands use this directory from the project or its subdirectories. An explicit `CONVO_CLI_HOME` takes precedence.

`doctor` checks the stored Signin credential and refreshes it when expired. A rejected refresh requires login again; network failures report `UNKNOWN`. Refresh-token revocation is detected when a refresh is attempted.

## Room and user identity

The companion server generates per-session room, user, RTC Token and task identities. `convo-cli token issue` supports GetToken; `--write` persists derived project identity. VoiceChat uses API version `2025-05-01`, `UserID` and `TargetUserID`. The server invokes `convo-cli agent start/stop`. Reconnect after changing scene configuration. `agent update` is currently unavailable.

## Development and diagnostics

Project files are `byteplus-rtc.config.yaml`, `byteplus-rtc.taskfile.yaml`, and `byteplus-rtc.template.yaml`. Restrict `.env.local` to the current user.

Run `convo-cli agent check` after the first `dev` setup to check provider credentials and Seed Speech access. The Demo runs preflight when settings change and before joining a room. Akool avatars require `AKOOL_API_KEY`.

Use `convo-cli doctor project` to check configuration and `convo-cli dev --auto-port` for local execution. Set `PORT` for the web app and `SERVER_PORT` for the API in `.env.local`. Exported environment variables take precedence over the file. Use `--web-port` and `--server-port` to override either setting for one run. `--dry-run` validates planned execution.

`dev` checks the Node/npm versions against the project's `package.json` engines and installs missing dependencies in the root, web, and server packages. Packages without dependencies can omit `node_modules`.

`dev` reports `ready` when the backend health check, web page, and proxied runtime configuration pass. CRA projects also wait for compilation and TypeScript checks. Vite TypeScript projects run their web `typecheck` script before starting services and wait for Vite's startup log and local entry-module responses. Vite transform errors remain compilation failures until the affected modules load successfully. `--startup-timeout` sets the limit for startup checks and web rebuilds (default `2m`). Startup failure or a later health failure lasting five seconds stops the command with a nonzero exit code.

| Request | Purpose |
| --- | --- |
| `GET /health` on the server port | Reports `ok` and `agentMode` (`cli` or `openapi`). |
| `GET /runtimeConfig` | Returns the public AppID in `Result.appId`; also available through the web proxy. |
| `POST /preflight?action=StartVoiceChat` | Accepts a VoiceChat request and checks provider configuration and speech access; returns `ready` and `issues`. |

Diagnose runtime errors by RTC join, welcome message, ASR, LLM and TTS stages. Capture sanitized error codes and request IDs from the same task.

---
