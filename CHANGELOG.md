# Changelog

## 0.0.1-rc.2

### Improvements

- Start the browser-configured Voice AI Demo from a scaffolded project or a compatible checkout. Choose Flexible or Realtime before dialing.
- Configure Flexible resources and the Realtime OpenAI API key independently during interactive resource setup.
- Pass a JSON request body through standard input with `openapi invoke --body @-`.

### Fixes

- Restrict the local Demo companion to approved browser origins and hosts.
- Use the configured endpoint and matching credentials for custom models and speech gateways in browser-managed calls.
- Check the browser Demo's Web SDK declaration and recognize compatible CLI-enabled checkouts.
- Guard RTC application changes when a local signing key belongs to the previous application.
- Preserve backend cleanup failures when stopping the Demo on Windows.

## 0.0.1-rc.1

### Improvements

- Guide RTC, model endpoint, API key, and speech application selection with reusable project and authentication context.
- Support project-local authentication and preserve user configuration during resource setup.
- Scaffold the pinned Voice AI Web Demo and validate provider readiness before starting conversations.
- Synchronize the official BytePlus Skill and migrate previous Skill names.
- Document release candidate installation through the npm `next` channel and an exact version.

### Fixes

- Wait for the Web client's readiness and report companion-service startup failures.
- Preserve native process-group signal handling in the Node launcher.
- Reject internal domains in public-source scans even when a line has an allow marker.
- Refresh model and speech credentials when resources are reselected, and use selected resource IDs during provider checks.
- Guide account recovery for resource permission errors and save RTC configuration before provider checks.

## 0.0.1-beta.1

### Improvements

- Scaffold the CLI-integrated Voice AI Demo from commit `7889444` with Web SDK 4.69.3.
- Check provider credentials and Seed Speech access before joining a conversation, and report missing Akool credentials before calling.
- Guide model endpoint, API key resource, and speech application selection while preserving local dotenv contents.
- Package six platform binaries and official Skills with the matching beta version and update channel.

### Fixes

- Discover RTC applications through the BytePlus Console API.
- Preserve the selected model endpoint and hydrate credentials for the selected ASR, LLM, TTS, and avatar providers.
- Group repeated conversation errors and clean up sessions after ASR reconnection attempts are exhausted.

## Unreleased

### Improvements

- Use English for repository entry documents, maintained specifications, CLI guidance, and BytePlus error-code guidance.
- Build and test BytePlus contracts through the standard Go and CI targets.
- Maintain convo-cli as a standalone BytePlus RTC conversational AI repository.
- Retain BytePlus RTC application setup, resource discovery, GetToken, VoiceChat request encoding, credential isolation, and pinned templates.
- Standardize the official Skill name as `byted-bp-convo-guide` and migrate previous names during synchronization.
- Use the BytePlus profile for installation packages, builds, documentation, and public entry checks.

### Fixes

- Emit registered BytePlus error codes without rewriting command data, and propagate output write failures.
- Persist dotenv values with private permissions and atomic replacement.
- Verify installer candidates before replacing existing Linux and macOS executables.
- Validate Skill examples against the configured CLI identity.
- Apply the local comparison fallback when the remote default branch has unrelated history.
- Use BytePlus documentation as the source for offline error codes and mark entries awaiting verification as unverified.
