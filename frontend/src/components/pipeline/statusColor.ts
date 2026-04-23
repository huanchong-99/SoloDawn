// E10-06: Shared status → Tailwind-class maps used by pipeline nodes and
// other terminal/task status indicators. Kept in one place so adding a new
// status only requires one edit.
//
// E10-07 (remaining): the literal `green-*`/`red-*`/`blue-*` classes are
// interim. Swap to design-system tokens (`success`/`error`/`warning`) once
// the new-design palette exposes matching Tailwind variants. The logical
// names (`running`, `failed`, …) stay stable.

export function getTerminalNodeClasses(status: string): string {
  switch (status) {
    case 'running':
    case 'working':
      return 'border-green-500 bg-green-500/10';
    case 'waiting':
      return 'border-blue-500 bg-blue-500/10';
    case 'completed':
      return 'border-gray-400 bg-gray-400/10';
    case 'failed':
      return 'border-red-500 bg-red-500/10';
    case 'starting':
      return 'border-yellow-500 bg-yellow-500/10';
    default:
      return 'border-border bg-secondary';
  }
}

export function getTerminalBadgeClasses(status: string): string {
  if (status === 'running' || status === 'working') {
    return 'bg-green-500/20 text-green-600';
  }
  if (status === 'failed') {
    return 'bg-red-500/20 text-red-600';
  }
  if (status === 'completed') {
    return 'bg-gray-500/20 text-gray-600';
  }
  return 'bg-blue-500/20 text-blue-600';
}
