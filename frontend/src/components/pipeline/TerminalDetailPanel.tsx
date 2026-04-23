interface TerminalDetailPanelProps {
  role?: string;
  status?: string;
  model?: string;
}

export function TerminalDetailPanel({ role, status, model }: Readonly<TerminalDetailPanelProps>) {
  // E10-05: Fall back to safe defaults when props are missing/undefined so the
  // panel never renders "undefined" into the DOM.
  const displayRole = role ?? 'Terminal';
  const displayStatus = status ?? 'unknown';
  const displayModel = model ?? 'n/a';

  return (
    <div className="p-3 bg-panel border border-border rounded">
      <div className="text-sm font-semibold">{displayRole}</div>
      <div className="text-xs text-low">Status: {displayStatus}</div>
      <div className="text-xs text-low">Model: {displayModel}</div>
    </div>
  );
}
