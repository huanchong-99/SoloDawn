interface OrchestratorHeaderProps {
  name: string;
  status: string;
  model: string | null;
  tokensUsed?: number | null;
}

/**
 * Format token count for display
 * e.g., 12500 -> "12.5k", 1500000 -> "1.5M"
 */
function formatTokens(tokens: number | null | undefined): string {
  if (tokens == null) return 'N/A';
  if (tokens >= 1_000_000) {
    return `${(tokens / 1_000_000).toFixed(1)}M`;
  }
  if (tokens >= 1_000) {
    return `${(tokens / 1_000).toFixed(1)}k`;
  }
  return tokens.toString();
}

export function OrchestratorHeader({ name, status, model, tokensUsed }: Readonly<OrchestratorHeaderProps>) {
  // TODO(E10-10): Localize hardcoded "Status:", "Model:", "Tokens Used",
  // "N/A", and "n/a" strings via i18n (workflow namespace) to match the rest
  // of the orchestrator UI.
  return (
    <div className="h-16 bg-panel border-b border-border px-6 flex items-center">
      <div className="flex-1">
        <div className="text-lg font-semibold">{name}</div>
        <div className="text-xs text-low">Status: {status} | Model: {model ?? 'n/a'}</div>
      </div>
      <div className="text-right text-xs">
        <div>Tokens Used</div>
        <div className="text-sm font-semibold">{formatTokens(tokensUsed)}</div>
      </div>
    </div>
  );
}
