# NexusIM Browser Multi-User UI Smoke

Date: 2026-06-23

Scope:

- Local private backend started by `loadtest/clientweb/run-local-smoke.ps1`.
- Browser / PC Web shell UI driven through two isolated Chromium profiles over CDP.
- Public client-facing path only:
  `api-gateway` HTTP BFF + `push-gateway` WebSocket.
- Verified path includes login, friend click to direct chat, UI group creation,
  group invite, direct / group send, PullInbox and AckDelivery.
- Not a capacity test, production SLO, TLS test, installer test or Android smoke.

Command:

```powershell
.\loadtest\clientweb\run-local-smoke.ps1 -RunName client-browser-multiuser-ui-smoke-20260623-170042 -RunBrowserMultiuserUISmoke -SkipBuild
```

Source summaries:

```text
H:\NexusIM\loadtest-results\client-browser-multiuser-ui-smoke-20260623-170042\client-web-summary.json
H:\NexusIM\loadtest-results\client-browser-multiuser-ui-smoke-20260623-170042\browser-multiuser-ui-smoke-summary.json
```

Clean baseline:

```text
commit=8782936b
git_dirty=false
client_web_success=true
browser_multi_user_ui_smoke=true
```

Key evidence:

- The runner injected the smoke BFF / push endpoints through the narrow shell
  config before loading the Web shell:
  `runtimeEndpointSource=fixture-shell-config`.
- Two isolated Chromium profiles were started:
  `senderBrowserStarted=true`, `receiverBrowserStarted=true`.
- Both users logged in through the rendered Web shell:
  `senderLoginOK=true`, `receiverLoginOK=true`.
- Direct chat through UI passed:
  - clicked the friend list to open direct chat;
  - `directConversationID=direct-b51551b03e8d02cd4f52302b34c979c2`;
  - `directMessageSeq=4`;
  - `directAckSeq=4`.
- Group chat through UI passed:
  - created group through the UI;
  - invited the receiver through group settings;
  - `groupConversationID=group-ab8b27c4-fc0b-4603-b621-451558113233`;
  - `groupMessageSeq=3`;
  - `groupAckSeq=3`.
- Browser runner verdict:
  - `directChatThroughUI=true`;
  - `groupChatThroughUI=true`;
  - `groupInviteThroughUI=true`;
  - `receiverAckObserved=true`.
- Underlying clientweb summary also passed public BFF / push direct and group
  paths with `success=true`.

Validated fixes:

- `BFFClient.openDirectConversation` now supplies a generated direct-chat
  idempotency key when the UI does not pass one explicitly.
- The browser multi-user runner now injects its temporary BFF / push endpoints
  into the shell before navigation, so it tests the same local stack that the
  fixture started.
- Temporary browser-profile cleanup no longer hides the emitted smoke summary.

Known non-blocking cleanup:

- Repeated local smoke setup still prints a PostgreSQL error for existing
  `ck_conversations_title_length` in
  `migrations/postgres/conversation/000011_conversation_profile.sql`.
  The run completed successfully, but the migration should be made idempotent
  in a separate focused cleanup.

Limits:

- Android WebView login, Windows installer packaging, signing, public TLS and
  production HA were not part of this run.
- This smoke proves a local two-browser UI flow, not capacity or long-running
  stability.
