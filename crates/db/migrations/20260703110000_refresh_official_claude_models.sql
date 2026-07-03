-- Refresh the official Claude Code subscription models to the 2026-07 lineup.
-- The seeded rows still pinned 2025-era ids: Opus 4.5 / Haiku 4.5 in their
-- retired dated forms (claude reports "model unavailable" and stalls), and
-- Sonnet 4.6 one generation behind. Each UPDATE is guarded on the previous
-- seeded value so a user-edited api_model_id is never clobbered.
-- Current ids (no date suffixes): claude-sonnet-5 / claude-opus-4-8 /
-- claude-haiku-4-5 / claude-fable-5.

UPDATE model_config
SET api_model_id = 'claude-sonnet-5',
    updated_at = datetime('now')
WHERE id = 'model-claude-sonnet'
  AND api_model_id IN ('claude-sonnet-4-6', 'claude-sonnet-4-20250514');

UPDATE model_config
SET api_model_id = 'claude-opus-4-8',
    updated_at = datetime('now')
WHERE id = 'model-claude-opus'
  AND api_model_id = 'claude-opus-4-5-20251101';

UPDATE model_config
SET api_model_id = 'claude-haiku-4-5',
    updated_at = datetime('now')
WHERE id = 'model-claude-haiku'
  AND api_model_id = 'claude-haiku-4-5-20251001';

-- New official option: Claude Fable 5 (Anthropic's most capable GA model).
-- Not the default — availability depends on the user's subscription plan.
INSERT OR IGNORE INTO model_config (id, cli_type_id, name, display_name, api_model_id, is_default, is_official)
VALUES ('model-claude-fable', 'cli-claude-code', 'fable', 'Claude Fable', 'claude-fable-5', 0, 1);
