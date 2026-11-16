<a id="english"></a>

# Support

## Usage questions

Start with the [README](./README.md),
[voice-agent guide](./docs/voice-agent.md#english), and
[troubleshooting guide](./docs/troubleshooting.md#english). Run `convo-cli doctor` before
requesting help; its checks usually identify the next action.

If the documentation does not answer the question, open an issue in this repository and
describe what you are trying to accomplish.

## Bug reports

Open the repository’s Issues page and create a bug report.

Include:

- `convo-cli version --format pretty` output;
- operating system and architecture;
- installation method;
- the exact command and exit code;
- a minimal reproduction;
- the redacted JSON error envelope or relevant `doctor` checks;
- expected and actual behavior.

Search existing issues before filing a duplicate. Never attach `.env.local` or
unredacted Console output.

## Feature requests

Open an issue in this repository describing the user problem, desired workflow, scene and
platform, and why existing commands do not cover it. The public command surface
is intentionally kept small, so proposals should focus on an end-to-end
developer outcome rather than an isolated flag.

## Security reports

Do not open a public issue for a vulnerability. Follow the private reporting
process in [SECURITY.md](./SECURITY.md#english).

---
