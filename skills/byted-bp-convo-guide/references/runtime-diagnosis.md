# Runtime diagnosis

Run `convo-cli doctor --format json` and preserve the returned error code and details.

Check the flow in order:

1. The browser joined with the expected AppID, room ID, user ID, and RTC token.
2. StartVoiceChat returned success and the agent joined the same room.
3. The welcome message played.
4. Microphone capture and upstream audio are active.
5. ASR is enabled for the selected speech resource and emits recognition results.
6. The LLM request completes.
7. TTS produces audio and the remote stream is subscribed and audible.
8. StopVoiceChat completes when the session ends.

A welcome message proves agent startup and downstream audio. If later turns receive no response, inspect microphone publication and ASR first. Use `convo-cli explain-error <code>` for known RTC and VoiceChat errors. Keep provider credentials out of copied diagnostics.
