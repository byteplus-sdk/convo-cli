<a id="english"></a>

# Security Policy

## Supported versions

Before the first stable release, security fixes are applied to the latest
published `0.x` release only. After a stable release, this section will list the
supported release lines explicitly.

## Reporting a vulnerability

Do not report suspected vulnerabilities in a public issue, discussion, pull
request, or chat. Use GitHub's private vulnerability reporting for this
repository:

Open this repository’s **Security** tab and select **Report a vulnerability**.

Include, when available:

- the affected version, platform, and installation method;
- the vulnerable command or workflow;
- reproduction steps or a minimal proof of concept;
- the security impact and any known prerequisites;
- suggested mitigations or patches;
- whether the issue has been disclosed anywhere else.

Do not include real production credentials. Use synthetic values and redact
AppKeys, access keys, Signin tokens, RTC tokens, private endpoints, account
identifiers, and Console data.

Maintainers will acknowledge the report as soon as practical, investigate it,
and coordinate remediation and disclosure with the reporter. Please allow time
for a fix and release before publishing details.

## Security boundaries

- Signin access and refresh tokens use the protected user-level
  `$CONVO_CLI_HOME/auth.json` file by default. The CLI uses the operating-system
  credential store only after the user explicitly selects `--store=keyring`.
- `RTC_APP_KEY` may be stored in the gitignored `.env.local` file but must not
  enter `byteplus-rtc.config.yaml`, `VITE_*` variables, frontend code, logs, or command
  output.
- Remote release and template artifacts are verified against pinned SHA-256
  digests before use.
- Generated projects and scene configuration should be reviewed before they are
  shared or deployed.

---
