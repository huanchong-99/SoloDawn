-- Sonnet 4 (`claude-sonnet-4-20250514`) was retired 2026-06-15. The seeded
-- official model `model-claude-sonnet` still pinned that id, so orchestrator
-- Claude Code terminals launched on a retired model and stalled with
-- "model unavailable" (claude reports the model retired/unavailable and waits at
-- a /model prompt). Bump it to the current Sonnet (`claude-sonnet-4-6`), which
-- the subscription endpoint accepts — this mirrors the orchestrator's native
-- fallback model in `agent.rs` / `llm.rs`.
--
-- Guarded on the old id so a user who already re-pointed this model is untouched.
UPDATE model_config
SET api_model_id = 'claude-sonnet-4-6',
    updated_at = CURRENT_TIMESTAMP
WHERE id = 'model-claude-sonnet'
  AND api_model_id = 'claude-sonnet-4-20250514';
