# VoiceChat API

The CLI signs BytePlus RTC OpenAPI requests for region `ap-southeast-1`.

- Start: `Action=StartVoiceChat`, `Version=2025-05-01`
- Stop: `Action=StopVoiceChat`, `Version=2025-05-01`
- RTC room token: signed `GetToken`, `Version=2025-06-01`, when no local AppKey is supplied
- Update: unsupported by the current product contract

Use `convo-cli dev` for the managed project flow. Use lower-level commands only when the user needs to inspect or control an existing task. Preserve the field casing returned by BytePlus documentation and the CLI configuration schema.

Confirm evolving request fields against [the official VoiceChat API documentation](https://docs.byteplus.com/en/docs/byteplus-rtc/docs-1558163) through `convo-cli docs` before presenting them as current.
