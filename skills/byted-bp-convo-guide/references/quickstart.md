# Quickstart

Carry project creation, authentication, resource selection and startup through as one workflow. Reuse the user's project path and existing choices. Continue through authorized steps as results become available.

## Authentication and project context

For an existing project, enter its directory first. Check authentication before downloading a new project or discovering resources:

```bash
convo-cli doctor cli --format json
convo-cli auth status --format json
```

Inspect `cli.auth`, `data.authenticated`, `data.expired`, and any error envelope. `doctor cli` can report unreadable credentials as a warning; `auth status` exposes storage and refresh failures. When `cli.auth` reports environment AK/SK fallback, retain that authentication method; `auth status` describes the Signin store. Resource discovery subsequently verifies account access to the required APIs.

For an account change, follow [account changes](resource-setup.md#account-changes-and-permission-failures) before reusing saved resource selections or RTC credentials.

When login is needed, complete browser authorization with `convo-cli auth login`. Reuse existing authorization to open the browser with `--browser=open`. Route storage errors to a writable credential directory and refresh network errors to connectivity recovery.

Keep the chosen absolute project path and authentication context on every subsequent command. When using `CONVO_CLI_HOME`, resolve it to an absolute path and supply it on every CLI invocation, including `dev`; shell environments may differ between tool calls. For project-local storage, create or enter the project and run `convo-cli auth login --project-auth`. This saves `auth.project: true`, which later commands in the project and its subdirectories reuse.

## Configure and start

Create the project when needed:

```bash
convo-cli init --scene voice-agent --platform web --name my-agent
cd my-agent
```

`init` downloads the pinned official `RTC_AIGC_Demo` source. For a clone of the [CLI-enabled Demo](https://github.com/byteplus-sdk/RTC_AIGC_Demo/tree/feat/rtc-cli-integration), run the resource and startup commands below from the repository root.

Follow [resource setup](resource-setup.md) for the selected mode. Flexible defaults to ModelArk and Seed Speech. Realtime uses an OpenAI API key. Carry the selected RTC AppID into startup:

```bash
convo-cli dev --app-id <selected-rtc-id>
```

`dev` saves the selected RTC AppID, starts the companion service, and launches the Web client. When the project already has the intended AppID, use `convo-cli dev`. Report startup success from the returned runtime status and URLs. Select providers in the Web client; their credentials are checked before starting a conversation.

The supported BytePlus VoiceChat contract uses API Version `2025-05-01` and has no BotID selection step. For selection requests, show the available resource names and use the chosen candidate's ID.

Use `--dry-run` before any setup operation when the user asks to inspect planned writes.

In a terminal, `agent resources --write` lets the user configure Flexible resources, enter the OpenAI API key at a hidden prompt, or do both. It saves the selections in `.env.local`. Select the mode in the Web client before dialing. Flexible checks the selected providers, including Seed Speech access when configured. Akool avatars require `AKOOL_API_KEY`.
