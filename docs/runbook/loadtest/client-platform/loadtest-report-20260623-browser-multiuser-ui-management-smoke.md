# NexusIM Browser Multi-User UI Management Smoke

Date: 2026-06-23

Scope:

- Local private backend started by `loadtest/clientweb/run-local-smoke.ps1`.
- Browser / PC Web shell UI driven through two isolated Chromium profiles over
  CDP.
- Public client-facing path only:
  `api-gateway` HTTP BFF + `push-gateway` WebSocket.
- Verified path includes login, friend click to direct chat, UI group creation,
  group invite, direct / group send, conversation tag / draft / archive
  round-trip, PullInbox and AckDelivery.
- Not a capacity test, production SLO, TLS test, installer test or Android smoke.

Command:

```powershell
.\loadtest\clientweb\run-local-smoke.ps1 -BindHost 127.0.0.1 -ClientHost 127.0.0.1 -RunName client-web-ui-management-smoke-20260623-211736 -RunBrowserMultiuserUISmoke
```

Source summaries:

```text
H:\NexusIM\loadtest-results\client-web-ui-management-smoke-20260623-211736\client-web-summary.json
H:\NexusIM\loadtest-results\client-web-ui-management-smoke-20260623-211736\browser-multiuser-ui-smoke-summary.json
```

Clean baseline:

```text
commit=7e8a890b
git_dirty=false
client_web_success=true
browser_multi_user_ui_smoke=true
conversation_management_through_ui=true
```

Key evidence:

- The run used clean commit `7e8a890b4e9baa143c85f1d19c43a500c571ea46`.
- Two isolated Chromium profiles were started and driven through the rendered Web
  shell.
- Both users logged in through the rendered Web shell:
  `senderLoginOK=true`, `receiverLoginOK=true`.
- Direct chat through UI passed:
  - clicked the friend list to open direct chat;
  - `directConversationID=direct-d471714ee814c0bd121db29b305f44fd`;
  - `directMessageSeq=4`;
  - `directAckSeq=4`.
- Group chat through UI passed:
  - created group through the UI;
  - invited the receiver through group settings;
  - `groupConversationID=group-8839b77c-0462-4425-a86f-0a711e666744`;
  - `groupMessageSeq=3`;
  - `groupAckSeq=3`.
- Conversation management through UI passed:
  - `tag=ui-smoke`;
  - `draftSavedAndCleared=true`;
  - `archiveRoundTrip=true`.
- Browser runner verdict:
  - `directChatThroughUI=true`;
  - `groupChatThroughUI=true`;
  - `groupInviteThroughUI=true`;
  - `conversationManagementThroughUI=true`;
  - `receiverAckObserved=true`.
- Underlying `client-web-summary.json` also passed with `success=true`.

Validated fixes:

- Active conversation selection is exposed on the conversation button as
  `data-active=true`, giving the runner a stable target without depending on
  parent CSS state.
- Web / PC shell preserves the selected locally-created conversation while
  receipt-service summary projection catches up, rather than dropping the row
  from the visible conversation list.
- Web / PC shell ignores stale async status writes from older UI actions, so an
  old select-conversation completion cannot overwrite the current
  conversation-management status.

Limits:

- Android WebView login, Windows installer packaging, signing, public TLS and
  production HA were not part of this run.
- This smoke proves a local two-browser UI management path, not capacity or
  long-running stability.
