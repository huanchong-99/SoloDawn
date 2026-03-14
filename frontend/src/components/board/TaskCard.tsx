import { useDraggable } from '@dnd-kit/core';
import { CSS } from '@dnd-kit/utilities';
import { Terminal } from '@phosphor-icons/react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { CSSProperties, PointerEvent, MouseEvent } from 'react';
import type { WorkflowTaskDto } from 'shared/types';
import { TerminalDots } from './TerminalDots';

interface TaskCardProps {
  readonly task: WorkflowTaskDto;
  readonly workflowId: string;
}

export function TaskCard({ task, workflowId }: Readonly<TaskCardProps>) {
  const navigate = useNavigate();
  const { t } = useTranslation('workflow');
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id,
  });

  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition: isDragging ? undefined : 'transform 150ms ease',
    touchAction: 'none',
  };

  const handleDebugClick = (event: PointerEvent | MouseEvent) => {
    event.stopPropagation();
    navigate(`/debug/${workflowId}`);
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`relative bg-panel border border-border rounded p-3 pr-10 select-none ${
        isDragging ? 'opacity-70 ring-2 ring-brand/30 shadow-lg z-50' : ''
      }`}
      {...attributes}
      {...listeners}
    >
      <button
        type="button"
        className="absolute right-2 top-2 z-20 rounded border border-border bg-secondary p-1.5 text-high shadow-sm hover:bg-brand hover:text-on-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40"
        aria-label={t('taskCard.debugButton')}
        title={t('taskCard.debugButton')}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={handleDebugClick}
      >
        <Terminal className="h-4 w-4" />
      </button>
      <div className="cursor-grab active:cursor-grabbing">
        <div className="text-sm font-semibold">{task.name}</div>
        <div className="text-xs text-low">{task.branch}</div>
        <div className="mt-2">
          <TerminalDots
            terminalCount={task.terminals.length}
            terminals={task.terminals.map((term) => ({
              id: term.id,
              status: term.status as 'not_started' | 'starting' | 'waiting' | 'working' | 'completed' | 'failed' | 'cancelled' | 'review_passed' | 'review_rejected' | 'quality_pending',
            }))}
          />
        </div>
      </div>
    </div>
  );
}
