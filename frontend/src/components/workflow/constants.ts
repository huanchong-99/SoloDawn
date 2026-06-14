// ============================================================================
// Workflow Constants
// ============================================================================

/**
 * CLI type definitions for terminal configuration
 * Matches backend BaseCodingAgent enum from shared/types.ts
 */
export const CLI_TYPES = {
  'cli-claude-code': {
    id: 'cli-claude-code',
    label: 'Claude Code',
    description: 'Anthropic Claude Code CLI',
    icon: 'terminal',
  },
  'cli-gemini-cli': {
    id: 'cli-gemini-cli',
    label: 'Gemini CLI',
    description: 'Google Gemini CLI',
    icon: 'terminal',
  },
  'cli-codex': {
    id: 'cli-codex',
    label: 'Codex',
    description: 'OpenAI Codex CLI',
    icon: 'terminal',
  },
  'cli-amp': {
    id: 'cli-amp',
    label: 'Amp',
    description: 'Sourcegraph Amp CLI',
    icon: 'terminal',
  },
  'cli-cursor-agent': {
    id: 'cli-cursor-agent',
    label: 'Cursor Agent',
    description: 'Cursor IDE Agent',
    icon: 'terminal',
  },
  'cli-qwen-code': {
    id: 'cli-qwen-code',
    label: 'Qwen Code',
    description: 'Alibaba Qwen Code CLI',
    icon: 'terminal',
  },
  'cli-copilot': {
    id: 'cli-copilot',
    label: 'Copilot',
    description: 'GitHub Copilot CLI',
    icon: 'terminal',
  },
  'cli-droid': {
    id: 'cli-droid',
    label: 'Droid',
    description: 'Droid AI CLI',
    icon: 'terminal',
  },
  'cli-opencode': {
    id: 'cli-opencode',
    label: 'Opencode',
    description: 'Opencode CLI',
    icon: 'terminal',
  },
} as const;

/** CLI type ID */
export type CliTypeId = keyof typeof CLI_TYPES;

/** Git commit format template */
export const GIT_COMMIT_FORMAT = `<type>(<scope>): <subject>

<body>

<footer>

Co-Authored-By: Claude <noreply@anthropic.com>`;
