# convo-cli scene Skill template

A Skill is an Agent workflow that identifies user intent, runs the shortest
working path, and routes failures to relevant diagnostic references. Use English
for repository documentation, Skill instructions, and examples.

Keep the top-level `SKILL.md` focused on decisions, command order, and failure
routing. Store API fields, error-code tables, and detailed troubleshooting in
flat `references/` files loaded as needed.

```text
byted-<product-code>-<skill-name>/
├── SKILL.md
├── LICENSE
└── references/
    ├── <domain-a>.md
    └── <integration-flow>.md
```

## 1. Frontmatter

```yaml
---
name: byted-<product-code>-<skill-name>
description: Explain, build, run, and diagnose <scene> workflows with BytePlus RTC. Use when the user asks about <scene-specific intents and symptoms>.
version: 1.0.0
---
```

- Match `name` to the directory name. Use lowercase letters, digits, and hyphens.
- Describe user intents and distinguish adjacent scenes in `description`.
- Provide a semantic `version` and a root `LICENSE` file.
- Include license headers in source files when required by their license.
- Preserve technical identifiers, frontmatter keys, commands, and paths.

## 2. Top-level `SKILL.md`

### Triggers

List natural-language intents, typical symptoms, and the supported
`scene × platform` combinations. Route adjacent requests to the relevant Skill.

### Working modes and CLI availability

For documentation questions, explanations, design discussions, and diagnosis of
user-provided logs, load references and answer with available evidence.

For scaffolding, authentication, service startup, or automated checks, verify
CLI availability once before the first execution:

```bash
convo-cli version --format json
```

When executing CLI commands through a tool, attach the Skill's name and version
as process-scoped metadata:

```text
VE_SKILL_ID=<name>/<version> convo-cli <command>
```

Use values without `@`. Keep the metadata process-scoped and omit it from
user-facing command examples and explanations.

If the CLI is unavailable, identify the blocked operation and continue any
available documentation or diagnostic work. Ask for authorization before
installing the CLI.

### Shortest working path

Provide ordered public commands and a success signal for each step:

```bash
convo-cli init --scene <scene> --platform <platform> --name <project-name>
cd <project-name>
convo-cli auth login
convo-cli dev
```

Use interactive first-run configuration by default. In non-interactive sessions,
read `error.code`, `details`, and `hint` for the next action. `convo-cli doctor`
performs read-only diagnostics. Supply secrets through environment variables or
an authorized credential-management workflow; keep them out of configuration,
logs, and examples.

### Intent and stage routing

| Intent or symptom | Stage | Reference |
|-------------------|-------|-----------|
| API fields and invocation | Configuration | `references/<api-domain>.md` |
| Media failure | Join, capture, publish, subscribe, playback | `references/<media-domain>.md` |
| Unclear or cross-domain failure | End-to-end workflow | `references/<integration-flow>.md` |

### Diagnosis and completion

- For CLI failures, inspect `error.code` and `hint`, run `convo-cli doctor` when
  relevant, then retry after resolving the cause.
- For SDK codes, run `convo-cli explain-error <code>` and follow `domain` and
  `doctor_check`.
- Trace cross-domain failures by stage and investigate the first failed stage
  or missing evidence.
- Verify the main workflow and related doctor checks before reporting success.

### Security boundaries and authoritative sources

State credential handling, user authorization, external side effects, and
internal-command boundaries. Link official sources and label unverified claims.

## 3. Progressive references

- Use flat `references/<file>.md` paths supported by the scanner and
  `convo-cli skills read`.
- Give each reference one reusable domain; compose domains through links.
- Place detailed API fields, event evidence, diagnostics, and long examples here.
- Resolve all local links within the Skill root.
- Verify command availability with `convo-cli <command> --help`, including its
  `when/avoid/prereq/examples` affordances.

## 4. Lifecycle notices

Complete and verify the current task before surfacing lifecycle notices. Run
updates after explicit user authorization. Route `_notice.update` to
`convo-cli update` and `_notice.skills` to `convo-cli skills sync`. Managed
automation may set `CONVO_CLI_NO_UPDATE_NOTIFIER=1` and
`CONVO_CLI_NO_SKILLS_NOTIFIER=1`.

## 5. Pre-submission checks

- The top-level workflow supports trigger decisions, execution, and failure routing.
- References are reusable and their links resolve.
- Examples use synthetic values and keep secrets out of conversation.
- The default workflow uses public commands; label hidden or legacy fallbacks.
- Documentation assistance remains available when the CLI is absent.
- Run the Skill quality gate for commands, metadata, links, secrets, and terminology.
