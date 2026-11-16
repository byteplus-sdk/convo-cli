---
name: byted-bp-convo-guide
description: Explain, integrate, run, and diagnose BytePlus RTC conversational AI and VoiceChat workflows. Use this Skill whenever a user asks about a BytePlus voice agent, StartVoiceChat, RTC_AIGC_Demo, convo-cli, room audio, ASR, LLM, TTS, or related BytePlus RTC documentation and errors.
version: "0.0.1-rc.1"
---

# BytePlus Conversational AI Guide

Route the request by stage, then load the matching reference. Keep commands and links within the BytePlus product surface.

## Workflow

1. For product capabilities and the shortest setup path, read `references/quickstart.md`.
2. For resource selection, account changes, or resource permission failures, read `references/resource-setup.md`.
3. For StartVoiceChat or StopVoiceChat request design, read `references/voicechat-api.md`.
4. For join, welcome-message, ASR, response, or audio failures, read `references/runtime-diagnosis.md`.
5. For current product facts or API details, read `references/documentation-retrieval.md` and verify them with `convo-cli docs`.

Before executing CLI operations, run:

```bash
convo-cli version --format json
```

If the command is unavailable and the user authorizes installation, run:

```bash
npm install -g @byteplus/convo-cli
```

For project setup, follow `references/quickstart.md` as one continuous workflow. Keep the project directory, authentication context and resource selections across commands. Collect unresolved resource choices together, then save and start within the user's requested scope. Pause for browser authorization, missing choices or a blocking error.

Read every JSON result's top-level `_notice`. Report relevant update or Skill synchronization notices after completing the active task. Respect `CONVO_CLI_NO_UPDATE_NOTIFIER=1` and `CONVO_CLI_NO_SKILLS_NOTIFIER=1` in managed automation.

## Safety

- Keep AppKey, access keys, model keys, speech keys, and session tokens out of config, logs, and chat responses.
- Ask the user to complete browser authorization when account interaction is required.
- Use `convo-cli doctor` for read-only readiness checks before changing project state.
- Treat `UpdateVoiceChat` as unsupported by the current BytePlus CLI contract.

## Sources

- [BytePlus RTC conversational AI overview](https://docs.byteplus.com/en/docs/byteplus-rtc/docs-1310537)
- [BytePlus RTC VoiceChat API](https://docs.byteplus.com/en/docs/byteplus-rtc/docs-1558163)
- [RTC_AIGC_Demo](https://github.com/byteplus-sdk/RTC_AIGC_Demo)
