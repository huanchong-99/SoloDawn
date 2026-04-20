import { useEffect, useRef, useState } from 'react';
import type { TerminalDto } from 'shared/types';
import { TerminalDetailPanel } from './TerminalDetailPanel';
import { getTerminalBadgeClasses, getTerminalNodeClasses } from './statusColor';
import { cn } from '@/lib/utils';

interface TerminalNodeProps {
  terminal: TerminalDto;
  taskName?: string;
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
          getTerminalNodeClasses(terminal.status)
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
            getTerminalBadgeClasses(terminal.status)
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
