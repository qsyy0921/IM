# NexusIM Browser Multi-User UI Filter Smoke

Date: 2026-06-23

Scope:

- Local private backend started by `loadtest/clientweb/run-local-smoke.ps1`.
- Browser / PC Web shell UI driven through two isolated Chromium profiles over
  CDP.
- Public client-facing path only:
  `api-gateway` HTTP BFF + `push-gateway` WebSocket.
- Verified path includes login, friend click to direct chat, UI group creation,
  group invite, direct / group send, conversation tag / draft / archive
  round-trip, tag / draft / archived-only filter inclusion and exclusion,
  PullInbox and AckDelivery.
- Not a capacity test, production SLO, TLS test, installer test or Android smoke.

Command:

```powershell
.\loadtest\clientweb\run-local-smoke.ps1 -BindHost 127.0.0.1 -ClientHost 127.0.0.1 -RunName client-web-ui-filter-smoke-20260623-214048 -RunBrowserMultiuserUISmoke
```

Source summaries:

```text
H:\NexusIM\loadtest-results\client-web-ui-filter-smoke-20260623-214048\client-web-summary.json
H:\NexusIM\loadtest-results\client-web-ui-filter-smoke-20260623-214048\browser-multiuser-ui-smoke-summary.json
```

Clean baseline:

```text
commit=05b8aec6
git_dirty=false
client_web_success=true
browser_multi_user_ui_smoke=true
conversation_management_through_ui=true
```

Key evidence:

- The run used clean commit `05b8aec6b7805e497cf65494cf7357a84f3ab2ff`.
- Two isolated Chromium profiles were started and driven through the rendered Web
  shell.
- Both users logged in through the rendered Web shell:
  `senderLoginOK=true`, `receiverLoginOK=true`.
- Direct chat through UI passed:
  - clicked the friend list to open direct chat;
  - `directConversationID=direct-f6b72937df8e3d89f384a50951ac666a`;
  - `directMessageSeq=4`;
  - `directAckSeq=4`.
- Group chat through UI passed:
  - created group through the UI;
  - invited the receiver through group settings;
  - `groupConversationID=group-c37bb821-8328-4d62-a1fc-127c5004e530`;
  - `groupMessageSeq=3`;
  - `groupAckSeq=3`.
- Conversation management and filters passed:
  - `tag=ui-smoke`;
  - `tagFilterMatched=true`;
  - `tagFilterExcludedMissingTag=true`;
  - `draftSavedAndCleared=true`;
  - `draftOnlyFilterMatched=true`;
  - `draftOnlyFilterExcludedAfterClear=true`;
  - `archiveRoundTrip=true`;
  - `archivedOnlyFilterMatched=true`;
  - `archivedOnlyFilterExcludedAfterUnarchive=true`.
- Browser runner verdict:
  - `directChatThroughUI=true`;
  - `groupChatThroughUI=true`;
  - `groupInviteThroughUI=true`;
  - `conversationManagementThroughUI=true`;
  - `receiverAckObserved=true`.
- Underlying `client-web-summary.json` also passed with `success=true`.

Validated fixes:

- Selected-conversation preservation during receipt projection lag now respects
  the active conversation-list filters. The Web / PC shell no longer keeps an
  active local row visible when it does not match tag, draft-only or
  archived-only filters.
- The browser multi-user runner now proves both positive and negative filter
  behavior for tag, draft-only and archived-only views.

Limits:

- Android WebView login, Windows installer packaging, signing, public TLS and
  production HA were not part of this run.
- This smoke proves a local two-browser UI filter path, not capacity or
  long-running stability.
