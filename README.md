# convo-cli


`convo-cli` creates, runs, and diagnoses BytePlus RTC conversational AI projects.

## Install and verify

Install the release candidate:

```bash
npm install -g @byteplus/convo-cli@next
```

To install a specific candidate, use `npm install -g @byteplus/convo-cli@0.0.1-rc.1`.

After a stable release is published, install it with:

```bash
npm install -g @byteplus/convo-cli
```

Verify the installation and synchronize the official Skill:

```bash
convo-cli version
convo-cli skills sync
convo-cli skills list
```

The Skill list should contain `byted-bp-convo-guide`.

## Quick start

```bash
convo-cli auth login --browser=open
convo-cli init ./my-agent --scene voice-agent --platform web
cd ./my-agent
convo-cli agent resources --write
convo-cli dev
```

On the first `dev` run, the CLI reads the RTC applications available to the signed-in account and writes the selected AppID and AppKey to the local `.env.local`. IAM sub-users need RTC application read permissions, including `DescribeApps` and `DescribeAppKeys`.

The Signin session is stored at `~/.convo-cli/auth.json` by default. Keep AppKeys, access tokens, and provider credentials in local environment files or protected credential storage.

Use `convo-cli agent resources --write` to choose model and speech resources by name. Use `--dry-run` to preview configuration updates. For setup and account changes, see [resource setup](skills/byted-bp-convo-guide/references/resource-setup.md).

`agent check` checks the configured providers and Seed Speech access before creating a conversation task. The Demo checks its current selection when configuration changes and before joining a room.

## Diagnostics

```bash
convo-cli auth status
convo-cli doctor project
convo-cli explain-error <code>
```

Generated projects use `byteplus-rtc.config.yaml`, `byteplus-rtc.taskfile.yaml`, and `byteplus-rtc.template.yaml`.

Use `convo-cli auth login --project-auth` to store credentials in the project and reuse them across commands.

`dev` and `doctor project` check the Node/npm versions required by the project's `package.json`. Vite TypeScript projects run their web typecheck before startup. Use `dev --auto-port` to select available ports and `--startup-timeout` to set the startup deadline. See [development checks and local endpoints](docs/voice-agent.md#development-and-diagnostics).

## Development

```bash
make build
./bin/convo-cli version
make test
make ci
```

[Voice-agent projects](docs/voice-agent.md) · [Automation](docs/automation.md) · [Troubleshooting](docs/troubleshooting.md)

[Contributing](CONTRIBUTING.md#english) · [Support](SUPPORT.md#english) · [Security](SECURITY.md#english) · [License](LICENSE)
