# Documentation retrieval

Use the CLI documentation commands for current BytePlus RTC material:

```bash
convo-cli docs search "StartVoiceChat"
convo-cli docs fetch <document-id>
convo-cli docs list
```

Search first, then fetch the exact result. Use list for browsing the documentation tree. Attribute versioned API fields to the fetched official document and state uncertainty when the official page does not define a value.

The BytePlus build routes these commands to the fixed BytePlus RTC documentation service. Keep API versions, endpoints, console paths, and account assumptions aligned with the fetched BytePlus source.
