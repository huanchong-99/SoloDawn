import { DndContext, DragOverlay, type DragEndEvent, type DragStartEvent, useDroppable } from '@dnd-kit/core';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useWorkflow, useUpdateTaskStatus } from '@/hooks/useWorkflows';
import type { WorkflowTaskDto } from 'shared/types';
import { TaskCard } from './TaskCard';

interface WorkflowKanbanBoardProps {
  readonly workflowId: string | null;
}

interface Column {
  readonly id: string;
  readonly titleKey: string;
}

// G29-005: Client-side status transition whitelist to prevent unnecessary API calls.
// The backend is the source of truth and will reject invalid transitions,
// but this avoids round-trips for obviously invalid drags.
const VALID_STATUS_TRANSITIONS: Record<string, string[]> = {
  pending: ['running', 'cancelled'],
  running: ['review_pending', 'completed', 'failed'],
  review_pending: ['running', 'completed', 'failed'],
  completed: [],
  failed: ['pending', 'running'],
  cancelled: ['pending'],
};

/**
 * Kanban columns matching backend WorkflowTaskStatus enum:
 * pending, running, review_pending, completed, failed, cancelled
 */
const columns: Column[] = [
  { id: 'pending', titleKey: 'kanban.columns.pending' },
  { id: 'running', titleKey: 'kanban.columns.running' },
  { id: 'review_pending', titleKey: 'kanban.columns.review_pending' },
  { id: 'completed', titleKey: 'kanban.columns.completed' },
  { id: 'failed', titleKey: 'kanban.columns.failed' },
  { id: 'cancelled', titleKey: 'kanban.columns.cancelled' },
];

interface KanbanColumnProps {
  readonly column: Column;
  readonly tasks: WorkflowTaskDto[];
  readonly workflowId: string;
}

/**
 * Droppable column for the Kanban board
 */
function KanbanColumn({ column, tasks, workflowId }: Readonly<KanbanColumnProps>) {
  const { t } = useTranslation('workflow');
  const { setNodeRef, isOver } = useDroppable({ id: column.id });
  const columnTasks = tasks.filter((task) => task.status === column.id);

  return (
    <div
      ref={setNodeRef}
      data-testid={`kanban-column-${column.id}`}
      className={`bg-panel border border-border rounded p-4 transition-colors ${
        isOver ? 'ring-2 ring-brand/40 bg-brand/5' : ''
      }`}
    >
      <div className="text-sm font-semibold mb-3">
        {t(column.titleKey)}
        <span className="ml-2 text-low font-normal">({columnTasks.length})</span>
      </div>
      <div className="space-y-2 min-h-[100px]">
        {columnTasks.map((task) => (
          <TaskCard key={task.id} task={task} workflowId={workflowId} />
        ))}
      </div>
    </div>
  );
}

// G29-006: This component calls useWorkflow independently from Board.tsx
// and TerminalActivityPanel. Workflow data could be lifted to a shared
// context or passed as a prop to avoid redundant fetches.
export function WorkflowKanbanBoard({ workflowId }: Readonly<WorkflowKanbanBoardProps>) {
  const { t } = useTranslation('workflow');
  const { data: workflow, isLoading } = useWorkflow(workflowId ?? '');
  const updateTaskStatus = useUpdateTaskStatus();

  const tasks = workflow?.tasks ?? [];

  // E09-01: Keep a ref to the freshest tasks list so drag-end doesn't read a
  // stale closure when the mutation resolves between drag-start and drag-end.
  const tasksRef = useRef<WorkflowTaskDto[]>(tasks);
  tasksRef.current = tasks;

  // E09-06: Track the task currently being dragged so we can render it inside
  // a <DragOverlay> for smoother visual feedback.
  const [activeTask, setActiveTask] = useState<WorkflowTaskDto | null>(null);

  // TODO(E09-03..07): Address remaining optimization/race issues flagged in the
  // Kanban review (debounced mutations, dnd sensors, keyboard a11y, etc.).
  // TODO(E09-08): Add drag-end test coverage for WorkflowKanbanBoard (skipped
  //   here; tracked separately so handleDragEnd's status-transition whitelist
  //   and stale-closure protection are exercised in unit tests).
  // TODO(E09-09): Configure dnd-kit sensors (PointerSensor with activation
  //   constraint + KeyboardSensor) instead of relying on defaults, so click
  //   vs. drag intent is disambiguated and keyboard dragging works.
  // TODO(E09-10): Announce drag events via dnd-kit's `accessibility` prop
  //   (screenReaderInstructions / announcements) for a11y parity.

  const handleDragStart = ({ active }: DragStartEvent) => {
    const taskId = String(active.id);
    const task = tasksRef.current.find((item) => item.id === taskId) ?? null;
    setActiveTask(task);
  };

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    setActiveTask(null);
    if (!workflowId || !over) return;

    const taskId = String(active.id);
    const nextStatus = String(over.id);

    // Validate the target is a valid column
    if (!columns.some((column) => column.id === nextStatus)) return;

    // E09-01: Read the freshest task list from the ref instead of the stale
    // closure captured when this handler was created.
    const currentTasks = tasksRef.current;
    const task = currentTasks.find((item) => item.id === taskId);
    if (!task || task.status === nextStatus) return;

    // G29-005: Client-side transition validation to avoid unnecessary API calls
    const allowedTransitions = VALID_STATUS_TRANSITIONS[task.status];
    if (allowedTransitions && !allowedTransitions.includes(nextStatus)) return;

    // Trigger the mutation (optimistic update handled in the hook)
    updateTaskStatus.mutate({
      workflowId,
      taskId,
      status: nextStatus,
    });
  };

  if (!workflowId) {
    return <div className="p-6 text-low">{t('kanban.selectWorkflow')}</div>;
  }

  if (isLoading) {
    return <div className="p-6 text-low">{t('kanban.loading')}</div>;
  }

  if (tasks.length === 0) {
    const isAgentPlanned = workflow?.executionMode === 'agent_planned';

    return (
      <div className="flex h-full items-center justify-center p-6">
        <div className="max-w-lg rounded border border-dashed border-border bg-panel p-6 text-center">
          <div className="text-base font-semibold text-high">
            {t('kanban.emptyTitle')}
          </div>
          <div className="mt-2 text-sm text-low">
            {isAgentPlanned
              ? t('kanban.emptyDescriptionAgentPlanned')
              : t('kanban.emptyDescription')}
          </div>
        </div>
      </div>
    );
  }

  return (
    <DndContext onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className="flex-1 p-6 grid grid-cols-6 gap-4">
        {columns.map((column) => (
          <KanbanColumn
            key={column.id}
            column={column}
            tasks={tasks}
            workflowId={workflowId}
          />
        ))}
      </div>
      {/* E09-06: Render the dragged item in an overlay for smoother feedback. */}
      <DragOverlay>
        {activeTask ? (
          <TaskCard task={activeTask} workflowId={workflowId} />
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}
