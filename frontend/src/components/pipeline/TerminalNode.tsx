import { useEffect, useRef, useState } from 'react';
import type { TerminalDto } from 'shared/types';
import { TerminalDetailPanel } from './TerminalDetailPanel';
import { cn } from '@/lib/utils';

interface TerminalNodeProps {
  terminal: TerminalDto;
  taskName?: string;
}

// TODO(E10-06): Extract status -> classes map to a shared module (shared with
// StatusBar/TaskCard) so terminal/task status styling stays in sync. The
// duplication across components risks drift when statuses are added.
// TODO(E10-07): Replace hardcoded Tailwind status colors (green/red/yellow)
// with design-system tokens (success/error/warning) once the tokens exist
// for the new-design palette.

/**
 * Get status color classes for terminal node
 */
function getStatusClasses(status: string): string {
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

/**
 * Get status badge classes for terminal node
 */
function getStatusBadgeClasses(status: string): string {
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

export function TerminalNode({ terminal, taskName }: Readonly<TerminalNodeProps>) {
  // E10-02: Ensure expanded state doesn't leak across unmount. We reset the
  // value via a cleanup effect so a remount starts collapsed.
  const [expanded, setExpanded] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // E10-01: Close the expanded detail panel when the user clicks outside of it.
  useEffect(() => {
    if (!expanded) return;
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (containerRef.current && target && !containerRef.current.contains(target)) {
        setExpanded(false);
      }
    };
    document.addEventListener('pointerdown', handlePointerDown);
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown);
    };
  }, [expanded]);

  // E10-02: State persistence cleanup on unmount.
  useEffect(() => {
    return () => {
      setExpanded(false);
    };
  }, []);

  return (
    <div ref={containerRef} className="relative">
      <button
        className={cn(
          'w-36 h-24 rounded border-2 flex flex-col items-center justify-center gap-1 transition-colors hover:bg-secondary/80',
          getStatusClasses(terminal.status)
        )}
        onClick={(event) => {
          // E10-03: Prevent the click from bubbling to outer handlers (e.g.,
          // pipeline canvas selection) and from triggering our own outside
          // click detector before state updates.
          event.stopPropagation();
          setExpanded((prev) => !prev);
        }}
      >
        <div className="text-xs font-medium">{terminal.cliTypeId}</div>
        {terminal.role && (
          <div className="text-[10px] text-low truncate max-w-[120px] px-1">
            {terminal.role}
          </div>
        )}
        <div
          className={cn(
            'text-xs px-2 py-0.5 rounded-full',
            getStatusBadgeClasses(terminal.status)
          )}
        >
          {terminal.status}
        </div>
      </button>

      {expanded && (
        <div className="absolute top-full mt-2 z-10">
          <TerminalDetailPanel
            role={terminal.role ?? taskName ?? 'Terminal'}
            status={terminal.status}
            model={terminal.modelConfigId}
          />
        </div>
      )}
    </div>
  );
}
