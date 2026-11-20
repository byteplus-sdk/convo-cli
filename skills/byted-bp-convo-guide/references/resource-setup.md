# RTC, model and speech resource setup

## Select resources

When the RTC application is unresolved, discover it before asking for resource choices through the existing OpenAPI command:

```bash
convo-cli openapi invoke --action DescribeApps --version 2018-01-01 --method GET --query Offset=0 --query Limit=100 --query ProjectName=default --query UserName=vcloud --query Platform=0 --query Keyword= --format json
```

Capture the response privately and expose only `AppId`, `Name`, and activity status from `data.body.Result.AppList`. The response may contain credentials. Accept applications whose `Status` or fallback `InstanceStatus` is `1` (string or number). Page with `Offset += 100` until a short page or a page with no new AppIDs, with a limit of 100 pages. Deduplicate by AppID. Keep the RTC query's `ProjectName=default` separate from the model and speech `--resource-project`.

Reuse an explicitly selected RTC AppID or the project's configured application. For an unresolved RTC choice, include the active applications with the provider choices below. Use names for selection and AppIDs to distinguish duplicate names. If no active application is available, report the account/query context and request access to an RTC application. The browser-configured Demo uses `DescribeApps` for discovery and `GetToken` for room tokens.

For Realtime, the user can enter their OpenAI API key at the hidden prompt in an interactive `convo-cli agent resources --write` session. They can skip Flexible resource configuration. In automation, supply `OPENAI_API_KEY` in the local environment.

For a custom LLM endpoint, set the HTTPS endpoint in `CUSTOM_LLM_URL` and its credential in `CUSTOM_LLM_API_KEY` locally. The browser endpoint must match the configured URL. The CLI uses `OPENAI_API_KEY` for the OpenAI endpoint and `GOOGLE_LLM_API_KEY` for the Google endpoint.

For Flexible with ModelArk and Seed Speech:

1. After authentication, run `convo-cli agent resources --format json`. Use the same `--resource-project` for discovery and saving.
2. Read `data.choices.model_endpoints`, `model_api_keys`, and `speech_apps`. Each candidate has `id`, `name`, and `display_name`. The query includes the project, region, and status filters; `data.discovery` reports each resource group's status.
3. Collect unresolved RTC, model service, model API key, and speech application choices in one question, using their names or `display_name`. Explain their purposes as needed: generating answers, authorizing model access, and handling speech. Use these names in questions and completion messages. Map each choice to its returned `id`.
4. Reuse the user's explicit selections. Otherwise, ask which resources to use, including when a group has one candidate. For duplicate or missing names, use the CLI's display labels and clarify ambiguous replies.
5. Save the selected IDs:

```bash
convo-cli agent resources --write --resource-project <project> --model-endpoint-id <selected-model-id> --model-api-key-id <selected-key-id> --speech-app-id <selected-speech-id> --format json
```

The CLI checks the provider selections again and saves them in `.env.local`. Continue to `convo-cli dev --app-id <selected-rtc-id>` when startup is within the authorized task. Keep the same project directory and authentication context throughout.

In a terminal, `convo-cli agent resources --write` accepts a number or name. For Agent calls, `byteplus-rtc.dev.selection_required` returns all groups in `error.details.choices` and the current group's candidates in `error.details.candidates`. Ask the user to choose from these labels, then retry with the mapped IDs. If a resource has disappeared, query again and ask for a replacement.

## Query results

- `available`: show the candidates.
- `empty`: state which resource is missing and the query's project, region, and status filter. Link the setup guide below and query again after setup.
- `failed`: explain the reported error and retry after resolving it. Resource availability is unknown. Speech query errors appear in `data.speech_error`; other query errors use the CLI error envelope. Retain successful results and include the returned setup links.

| Resource | Guide and setup |
| --- | --- |
| Model service | [ModelArk online inference](https://docs.byteplus.com/en/docs/modelark/online-inference-standard): create a service in the target project and wait until it is ready. |
| Model API key | [Obtain an API key](https://docs.byteplus.com/en/docs/modelark/api-key): create a named key in the model service's project with access to that service. |
| Speech application | [Old Console Guide](https://docs.byteplus.com/en/docs/byteplusvoice/Speech_Console_Guide#E4ysfjvI): create an application in the [Seed Speech console](https://console.byteplus.com/voice/app) and activate ASR and TTS. The application provides App ID and Access Token after activation. |

## Account changes and permission failures

1. Keep the absolute project path and authentication storage context on every command. Stop the project's running dev process before changing accounts. Retain the user's authorization to configure and start the project.
2. Review RTC and provider variables in the shell and `.env.local` without exposing their values. Update or unset credentials belonging to the previous account through the local credential workflow. Shell variables take precedence over `.env.local`. The companion server uses configured `BYTEPLUS_ACCESS_KEY_ID` and `BYTEPLUS_SECRET_ACCESS_KEY`; CLI requests can use them as an authentication fallback.
3. Run `convo-cli auth login --force --browser=open` when switching Signin accounts. Ask the user to authorize the intended account in the browser. For environment-based authentication, use the intended account's access keys and verify access through resource discovery.
4. Repeat [resource selection](#select-resources) under the new authentication context. Discover RTC, model and speech resources, collect unresolved choices together by name, and save the chosen provider IDs with `convo-cli agent resources --write`. The command clears the local `BYTEPLUS_MODEL_API_KEY` and `BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN`; runtime requests obtain credentials for the selected resources. A selected model API key ID takes precedence over `BYTEPLUS_MODEL_API_KEY`.
5. Inspect `restart_required` and `next_steps` in the save result. When startup is authorized, run `convo-cli dev --app-id <selected-rtc-id>` with the newly selected RTC AppID to select that application and start the services. Report the returned runtime status and URLs. For projects with a saved agent scene, run `convo-cli agent check` from another terminal in the same project and authentication context after RTC configuration is saved. Resolve check failures before starting a conversation. The Demo checks its provider selection before joining a room.

For `AccessDenied.NoPermissionForApp`, select an RTC application accessible to the authenticated account or ask its owner or IAM administrator to grant access. `convo-cli dev --reconfigure` also supports selecting an RTC application again and refreshing its credentials in a terminal.

For an empty Project selector, ask the account administrator to check project visibility and permissions. For `ApplicationCredentials` HTTP 403, check Seed Speech application access with the account administrator. Check ASR and TTS activation with `convo-cli agent check`. Ask the user's organization or application owner to identify the account administrator when needed. Continue resource discovery after access is granted.
