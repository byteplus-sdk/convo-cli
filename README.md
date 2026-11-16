# convo-cli

`convo-cli` helps developers and Coding Agents create, run, and diagnose BytePlus RTC conversational AI projects. The current project template supports `voice-agent × web`.

[Install](#install-and-verify) · [Quick start](#quick-start) · [Commands](#commands) · [Agent and CI](#agent-and-ci) · [Documentation](#documentation)

## Install and verify

Requires Node.js 16 or later. Prebuilt binaries support macOS, Linux, and Windows on amd64 and arm64. The npm installer downloads the matching GitHub Release binary and verifies its SHA-256 checksum.

Install from npm:

```bash
npm install -g @byteplus/convo-cli
```

Available versions are listed in [GitHub Releases](https://github.com/byteplus-sdk/convo-cli/releases).

Verify the installation:

```bash
convo-cli version
```

## Quick start

Use a BytePlus account with RTC enabled. The default conversation providers use ModelArk and Seed Speech; see [resource setup](skills/byted-bp-convo-guide/references/resource-setup.md) for account permissions and provider choices.

```bash
convo-cli auth login --browser=open
convo-cli init ./my-agent --scene voice-agent --platform web
cd ./my-agent
convo-cli agent resources --write
convo-cli dev
```

On the first `dev` run, the CLI reads the RTC applications available to the signed-in account and writes the selected AppID and AppKey to the local `.env.local`. IAM sub-users need RTC application read permissions, including `DescribeApps` and `DescribeAppKeys`.

Open the web URL reported by `dev`, select the conversation providers, and start a conversation. Use `convo-cli dev --app-id <app-id>` to select an RTC application explicitly.

Use `convo-cli agent resources --write` to choose model and speech resources by name. Use `--dry-run` to preview configuration updates. For setup and account changes, see [resource setup](skills/byted-bp-convo-guide/references/resource-setup.md).

`agent check` checks the configured providers and Seed Speech access before creating a conversation task. The Demo checks its current selection when configuration changes and before joining a room.

## Commands

| Command | Purpose |
| --- | --- |
| `init [dir] --scene voice-agent --platform web` | Create a project from the pinned official template. |
| `auth login` / `auth status` / `auth logout` | Manage BytePlus account authorization. |
| `agent resources --write` / `agent check` | Select model and speech resources and check provider readiness. |
| `dev [--app-id <id>] [--auto-port]` | Configure RTC resources and run the Web app and companion service. |
| `doctor [cli\|project]` / `explain-error <code>` | Check readiness and look up SDK or conversational AI runtime errors offline. |
| `docs search/fetch/list` | Search and read current RTC documentation; `fetch --match` extracts matching sections and tables. |
| `skills list/read/sync` / `update --check` | Inspect official Skills and check for CLI updates. |

Run `convo-cli <command> --help` for prerequisites, options, and examples. Documentation queries require network access and work without project configuration or login.

## Diagnostics

```bash
convo-cli auth status
convo-cli doctor project
convo-cli explain-error <code>
```

`dev` and `doctor project` check the Node/npm versions required by the project's `package.json`. Vite TypeScript projects run their web typecheck before startup. Use `dev --auto-port` to select available ports and `--startup-timeout` to set the startup deadline. See [development checks and local endpoints](docs/voice-agent.md#development-and-diagnostics).

## Agent and CI

Install the official Skill:

```bash
npx skills add byteplus-sdk/convo-cli -g -y
```

To match the Skill to the installed CLI release or refresh an existing installation:

```bash
convo-cli skills sync
convo-cli skills list
```

The Skill list should contain `byted-bp-convo-guide`.

Agents and scripts should pass `--format json`. Command data goes to stdout; progress and warnings go to stderr. Failures return a nonzero exit code and a stable `error.code`. Use `--dry-run` to preview configuration and project changes.

See [automation and structured output](docs/automation.md) for headless authorization, resource selection, error handling, and lifecycle notices.

## Configuration and credentials

Generated projects use `byteplus-rtc.config.yaml`, `byteplus-rtc.taskfile.yaml`, and `byteplus-rtc.template.yaml`. Project configuration holds metadata and `${ENV}` references; `.env.local` holds runtime credentials and is Git-ignored.

The Signin session is stored at `~/.convo-cli/auth.json` by default. Use `convo-cli auth login --project-auth` to store credentials in the project and reuse them across commands. Keep AppKeys, access tokens, and provider credentials in local environment files or protected credential storage. See [Security](SECURITY.md#english) for credential handling and vulnerability reports.

## Documentation

[Voice-agent projects](docs/voice-agent.md) · [Resource setup](skills/byted-bp-convo-guide/references/resource-setup.md) · [Automation](docs/automation.md) · [Troubleshooting](docs/troubleshooting.md) · [Changelog](CHANGELOG.md)

## Development

Building from source requires Go 1.26.11 or later:

```bash
make build
./bin/convo-cli version
make test
make ci
```

[Contributing](CONTRIBUTING.md#english) · [Support](SUPPORT.md#english) · [Security](SECURITY.md#english) · [License](LICENSE)
