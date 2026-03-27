# Orchestrator Chat Rollback Runbook

## 1. Feature Flags

- `SOLODAWN_ORCHESTRATOR_CHAT_ENABLED`
  - `true` (default): enable workflow orchestrator chat (`/api/workflows/:id/orchestrator/*`)
  - `false`: disable orchestrator chat entrypoint and message listing
- `SOLODAWN_CHAT_CONNECTOR_ENABLED`
  - `true` (default): enable social/chat connector routes (`/api/integrations/chat/*`)
  - `false`: disable external connector bind/unbind/event ingestion

## 2. Emergency Rollback Procedure (No DB Change)

1. Set:
   - `SOLODAWN_ORCHESTRATOR_CHAT_ENABLED=false`
   - `SOLODAWN_CHAT_CONNECTOR_ENABLED=false`
2. Restart server process.
3. Verify:
   - Orchestrator chat calls return conflict with feature-disabled message.
   - Existing workflow session chat and workflow execution endpoints still operate normally.

## 3. Partial Rollback

- Disable only external channel:
  - keep `SOLODAWN_ORCHESTRATOR_CHAT_ENABLED=true`
  - set `SOLODAWN_CHAT_CONNECTOR_ENABLED=false`
- Result:
  - Web workflow orchestrator chat remains available.
  - Social provider ingress is blocked.

## 4. Data Migration Rollback (If Required)

The chat persistence migration introduced:

- `workflow_orchestrator_message`
- `workflow_orchestrator_command`
- `external_conversation_binding`

Rollback SQL (run only when you intentionally remove this feature set):

```sql
DROP TABLE IF EXISTS external_conversation_binding;
DROP TABLE IF EXISTS workflow_orchestrator_command;
DROP TABLE IF EXISTS workflow_orchestrator_message;
```

## 5. Post-Rollback Validation Checklist

1. `cargo test -p server --lib orchestrator_chat_route_tests`
2. `cargo test -p server --lib chat_integrations::tests`
3. `pnpm --dir frontend test:run src/pages/Workflows.test.tsx`
4. Open Workflows page and confirm:
   - Session chat path remains unaffected.
   - No orchestrator chat side effects when flags are disabled.
