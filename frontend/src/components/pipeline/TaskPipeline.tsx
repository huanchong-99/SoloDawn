import { useWorkflow } from '@/hooks/useWorkflows';
import { useTranslation } from 'react-i18next';
import { GitBranch, ArrowRight } from 'lucide-react';
import { TerminalNode } from './TerminalNode';
import { MergeTerminalNode } from './MergeTerminalNode';
import type { WorkflowTaskDto } from 'shared/types';

interface TaskPipelineProps {
  workflowId: string;
}

/**
 * Task column with branch info and terminals
 */
function TaskColumn({ task, isLast }: Readonly<{ task: WorkflowTaskDto; isLast: boolean }>) {
  const { t } = useTranslation('workflow');
  const terminals = task.terminals ?? [];

  return (
    <div className="flex items-start gap-4">
      <div className="flex flex-col gap-3 min-w-[140px]">
        {/* Task header with name and branch */}
        <div className="text-center space-y-1">
          <div className="text-sm font-semibold">{task.name}</div>
          <div className="flex items-center justify-center gap-1 text-xs text-low">
            <GitBranch className="w-3 h-3" />
            <span className="truncate max-w-[120px]" title={task.branch}>
              {task.branch}
            </span>
          </div>
          <div className="text-[10px] text-low">
            {t('pipeline.terminalsCount', { count: terminals.length })}
          </div>
        </div>

        {/* Terminal nodes.
            E10-04: `terminal.id` is used as the React key here and must be
            unique within a task. The backend (WorkflowTaskDto.terminals) is
            the source of truth and guarantees uniqueness per task; if that
            invariant is ever violated React will warn about duplicate keys.
            Explicit dedup is intentionally NOT done here so such backend
            bugs surface instead of being silently masked. */}
        {terminals.map((terminal, idx) => (
          <div key={terminal.id} className="relative">
            <TerminalNode terminal={terminal} taskName={task.name} />
            {/* Vertical connector between terminals */}
            {idx < terminals.length - 1 && (
              <div className="absolute left-1/2 -translate-x-1/2 top-full h-3 w-px bg-border" />
            )}
          </div>
        ))}
      </div>

      {/* Horizontal connector to next task */}
      {!isLast && (
        <div className="flex items-center self-center mt-16">
          <div className="w-6 h-px bg-border" />
          <ArrowRight className="w-4 h-4 text-low" />
        </div>
      )}
    </div>
  );
}

export function TaskPipeline({ workflowId }: Readonly<TaskPipelineProps>) {
  const { t } = useTranslation('workflow');
  const { data: workflow } = useWorkflow(workflowId);
  const tasks = workflow?.tasks ?? [];
  const commands = workflow?.commands ?? [];
  const isAgentPlanned = workflow?.executionMode === 'agent_planned';

  return (
    <div className="flex-1 p-6 overflow-x-auto">
      {/* Commands bar */}
      {commands.length > 0 && (
        <div className="mb-4 px-3 py-2 bg-secondary rounded border border-border">
          <div className="text-xs text-low mb-1">{t('pipeline.slashCommands')}</div>
          <div className="flex flex-wrap gap-2">
            {commands.map((cmd) => (
              <span
                key={cmd.id}
                className="px-2 py-0.5 bg-panel rounded text-xs font-mono"
                title={cmd.preset.description}
              >
                {cmd.preset.command}
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Pipeline visualization */}
      {tasks.length === 0 ? (
        <div className="rounded border border-dashed border-border bg-panel px-6 py-8 text-center">
          <div className="text-sm font-medium text-high">
            {t('pipeline.emptyTitle')}
          </div>
          <div className="mt-2 text-xs text-low">
            {isAgentPlanned
              ? t('pipeline.emptyDescriptionAgentPlanned')
              : t('pipeline.emptyDescription')}
          </div>
        </div>
      ) : (
        <div className="flex gap-2 min-w-max items-start">
          {tasks.map((task, idx) => (
            <TaskColumn
              key={task.id}
              task={task}
              isLast={idx === tasks.length - 1}
            />
          ))}

          {/* Arrow to merge terminal */}
          <div className="flex items-center self-center mt-16">
            <div className="w-6 h-px bg-border" />
            <ArrowRight className="w-4 h-4 text-low" />
          </div>

          <MergeTerminalNode workflowId={workflowId} />
        </div>
      )}

      {/* Bottom hint */}
      <div className="mt-6 text-xs text-low text-center">
        {t('pipeline.hint', { defaultValue: 'Click on a terminal to view details' })}
      </div>
    </div>
  );
}
