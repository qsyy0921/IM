# message-service mutation verified metadata auth smoke

Date: 2026-06-13

## Conclusion

This smoke verifies that the three user-facing message mutation RPCs can run with gateway verified identity metadata:

```text
SendMessage(metadata auth)
-> PullInbox(message.persisted.v1, metadata auth)
-> EditMessage / RevokeMessage / DeleteMessage(metadata auth)
-> message outbox relay
-> delivery timeline projection
-> PullInbox(message.edited.v1 / message.revoked.v1 / message.deleted.v1, metadata auth)
-> AckDelivery(metadata auth)
```

The result proves that message-service no longer depends on request-body identity for the mutation path when `NEXUSIM_MESSAGE_AUTH_MODE=metadata` is enabled. It also verifies that conversation-service and delivery-service user-facing RPCs in the same smoke can accept the same gateway verified metadata shape.

This is a local small smoke, not a capacity result and not a full API gateway or mTLS rollout.

## Commands

```powershell
.\loadtest\messageedit\run-local-smoke.ps1 `
  -VerifiedAuthMetadata `
  -RunName "message-edit-verified-metadata-smoke-20260613-190950"

.\loadtest\messagerevoke\run-local-smoke.ps1 `
  -VerifiedAuthMetadata `
  -RunName "message-revoke-verified-metadata-smoke-20260613-191010"

.\loadtest\messagedelete\run-local-smoke.ps1 `
  -VerifiedAuthMetadata `
  -RunName "message-delete-verified-metadata-smoke-20260613-191033"
```

## Raw Results

```text
H:\NexusIM\loadtest-results\message-edit-verified-metadata-smoke-20260613-190950\message-edit-summary.json
H:\NexusIM\loadtest-results\message-revoke-verified-metadata-smoke-20260613-191010\message-revoke-summary.json
H:\NexusIM\loadtest-results\message-delete-verified-metadata-smoke-20260613-191033\message-delete-summary.json
```

## Shared Baseline

All three runs used:

```text
commit=5357dfb
git_dirty=false
conversation_tls_enabled=false
message_tls_enabled=false
delivery_tls_enabled=false
verified_auth_metadata=true
success=true
conversation_target=127.0.0.1:11596
message_target=127.0.0.1:11595
delivery_target=127.0.0.1:11597
```

The scripts started local conversation-service, message-service, delivery-service, message outbox relay, delivery timeline consumer, and delivery outbox relay. Each scenario used an isolated `conversation.timeline.*` topic and a fresh delivery consumer group.

## Edit Evidence

```text
tenant_id=tenant-message-edit-smoke
conversation_id=conv-message-edit-smoke
send_message.conversation_seq=2
pull_persisted.item_count=1
pull_persisted.items[0].event_type=message.persisted.v1
edit_message.conversation_seq=3
edit_message.change_version=1
pull_edited.item_count=1
pull_edited.max_seq=3
pull_edited.items[0].event_type=message.edited.v1
ack_delivery.last_received_seq=3
message_log_status=EDITED
message_change_rows=1
message_change_history.change_type=EDIT
message_change_history.before_status=NORMAL
message_change_history.after_status=EDITED
timeline_event_counts.message.edited.v1=1
message_outbox_counts.PUBLISHED=3
user_inbox_event_counts.message.edited.v1=1
delivery_outbox_counts.PUBLISHED=3
cursor_last_received_seq=3
```

## Revoke Evidence

```text
tenant_id=tenant-message-revoke-smoke
conversation_id=conv-message-revoke-smoke
send_message.conversation_seq=2
pull_persisted.item_count=1
pull_persisted.items[0].event_type=message.persisted.v1
revoke_message.conversation_seq=3
revoke_message.change_version=1
pull_revoked.item_count=1
pull_revoked.max_seq=3
pull_revoked.items[0].event_type=message.revoked.v1
ack_delivery.last_received_seq=3
message_log_status=REVOKED
message_change_rows=1
timeline_event_counts.message.revoked.v1=1
message_outbox_counts.PUBLISHED=3
user_inbox_event_counts.message.revoked.v1=1
delivery_outbox_counts.PUBLISHED=3
cursor_last_received_seq=3
```

## Delete Evidence

```text
tenant_id=tenant-message-delete-smoke
conversation_id=conv-message-delete-smoke
send_message.conversation_seq=2
pull_persisted.item_count=1
pull_persisted.items[0].event_type=message.persisted.v1
delete_message.conversation_seq=3
delete_message.change_version=1
pull_deleted.item_count=1
pull_deleted.max_seq=3
pull_deleted.items[0].event_type=message.deleted.v1
ack_delivery.last_received_seq=3
message_log_status=DELETED
message_change_rows=1
message_change_history.change_type=DELETE
message_change_history.before_status=NORMAL
message_change_history.after_status=DELETED
timeline_event_counts.message.deleted.v1=1
message_outbox_counts.PUBLISHED=3
user_inbox_event_counts.message.deleted.v1=1
delivery_outbox_counts.PUBLISHED=3
cursor_last_received_seq=3
```

## Boundary

- `-VerifiedAuthMetadata` validates the gateway verified metadata interface shape for message mutations.
- Request body identity remains for the default body-auth compatibility mode.
- The smoke does not validate a production API gateway, certificate issuance, certificate rotation, or dynamic service identity.
- Edit/revoke/delete product semantics remain the existing first-stage semantics: sender-owned mutation unless policy ownership override is enabled through the policy path; this smoke only verifies the metadata-auth transport for the standard mutation runners.
