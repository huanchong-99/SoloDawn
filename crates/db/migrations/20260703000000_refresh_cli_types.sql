-- ============================================================================
-- CLI Type Refresh Migration
-- Created: 2026-07-03
-- Description: Align the cli_type seed data with the July 2026 state of the
--   supported AI CLIs (verified against official vendor sources):
--   1. Remove Gemini CLI — Google deprecated it for consumers on 2026-06-18
--      in favor of the closed-source Antigravity CLI. The row (and its
--      model_config rows via ON DELETE CASCADE) is deleted only when no
--      workflow/terminal ever referenced it, so historical databases keep
--      their referential integrity.
--   2. GitHub Copilot: the `gh copilot` extension stopped working on
--      2025-10-25; the standalone `copilot` binary replaced it.
--   3. Cursor: the agent binary is `cursor-agent` (`cursor` is the IDE);
--      docs moved from cursor.sh to cursor.com.
--   4. Droid / Opencode / Claude Code: refresh stale install-guide URLs.
-- ============================================================================

-- 1. Remove the Gemini CLI seed row where it was never used.
--    model_config rows cascade; guard against every FK that references
--    cli_type or a Gemini-owned model_config (terminal, workflow merge/error).
DELETE FROM cli_type
WHERE id = 'cli-gemini'
  AND NOT EXISTS (SELECT 1 FROM terminal WHERE cli_type_id = 'cli-gemini')
  AND NOT EXISTS (
      SELECT 1 FROM terminal t
      JOIN model_config mc ON t.model_config_id = mc.id
      WHERE mc.cli_type_id = 'cli-gemini'
  )
  AND NOT EXISTS (
      SELECT 1 FROM workflow
      WHERE merge_terminal_cli_id = 'cli-gemini'
         OR error_terminal_cli_id = 'cli-gemini'
  )
  AND NOT EXISTS (
      SELECT 1 FROM workflow w
      JOIN model_config mc ON mc.cli_type_id = 'cli-gemini'
      WHERE w.merge_terminal_model_id = mc.id
         OR w.error_terminal_model_id = mc.id
  );

-- 2. GitHub Copilot: standalone CLI replaced the dead gh extension.
UPDATE cli_type
SET detect_command = 'copilot --version',
    install_guide_url = 'https://docs.github.com/en/copilot/concepts/agents/about-copilot-cli'
WHERE id = 'cli-copilot';

-- 3. Cursor: detect the real agent binary; refresh docs URL.
UPDATE cli_type
SET detect_command = 'cursor-agent --version',
    install_guide_url = 'https://cursor.com/docs/cli'
WHERE id = 'cli-cursor';

-- 4. Droid: droid.dev was never Factory's domain.
UPDATE cli_type
SET install_guide_url = 'https://docs.factory.ai/cli'
WHERE id = 'cli-droid';

-- 5. Opencode: official site is opencode.ai (repo moved to anomalyco/opencode).
UPDATE cli_type
SET install_guide_url = 'https://opencode.ai/docs'
WHERE id = 'cli-opencode';

-- 6. Claude Code: docs moved to code.claude.com.
UPDATE cli_type
SET install_guide_url = 'https://code.claude.com/docs'
WHERE id = 'cli-claude-code';
