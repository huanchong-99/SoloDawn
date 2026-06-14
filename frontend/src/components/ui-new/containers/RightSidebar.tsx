import { GitPanelCreateContainer } from '@/components/ui-new/containers/GitPanelCreateContainer';
import { FeishuChannelContainer } from '@/components/ui-new/containers/FeishuChannelContainer';
import { FileTreeContainer } from '@/components/ui-new/containers/FileTreeContainer';
import { ProcessListContainer } from '@/components/ui-new/containers/ProcessListContainer';
import { PreviewControlsContainer } from '@/components/ui-new/containers/PreviewControlsContainer';
import { GitPanelContainer } from '@/components/ui-new/containers/GitPanelContainer';
import { useChangesView } from '@/contexts/ChangesViewContext';
import { useLogsPanel } from '@/contexts/LogsPanelContext';
import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
import { useOrchestrationDiff } from '@/contexts/OrchestrationDiffContext';
import type {
  Workspace,
  RepoWithTargetBranch,
  WorkflowDetailDto,
  WorkflowTaskDto,
} from 'shared/types';
import {
  ArrowSquareOutIcon,
  GitBranchIcon,
  GitDiffIcon,
} from '@phosphor-icons/react';
import {
  RIGHT_MAIN_PANEL_MODES,
  type RightMainPanelMode,
  useExpandedAll,
} from '@/stores/useUiPreferencesStore';

function workflowStatusClass(status: string): string {
  if (status === 'running' || status === 'completed') return 'bg-success/20 text-success';
  if (status === 'failed') return 'bg-error/20 text-error';
  return 'bg-tertiary text-low';
}

function taskStatusDotClass(status: string): string {
  if (status === 'completed') return 'bg-success';
  if (status === 'running') return 'bg-success animate-pulse';
  if (status === 'failed') return 'bg-error';
  return 'bg-tertiary';
}

function terminalStatusDotClass(status: string): string {
  if (status === 'working') return 'bg-success animate-pulse';
  if (status === 'completed') return 'bg-success';
  if (status === 'failed') return 'bg-error';
  if (status === 'waiting') return 'bg-brand';
  return 'bg-tertiary';
}

// FE-2: per-task acceptance-score badge color (0-100). Mirrors the audit
// scoring buckets used elsewhere; null until a review has run.
function scoreBadgeClass(score: number): string {
  if (score >= 80) return 'bg-success/20 text-success';
  if (score >= 60) return 'bg-brand/20 text-brand';
  return 'bg-error/20 text-error';
}

interface ConciergeTaskCardProps {
  readonly task: WorkflowTaskDto;
  readonly workflowId: string;
  readonly onViewChanges: (workflowId: string, taskId: string) => void;
}

// FE-2 (Phase C step 12): per-task card with a score badge + a "View Changes"
// affordance that sets the diff target and auto-opens the CHANGES panel, so the
// orchestration output is auditable per task (Q2 Break 5/6).
function ConciergeTaskCard({
  task,
  workflowId,
  onViewChanges,
}: ConciergeTaskCardProps) {
  const score = task.acceptanceScore;
  return (
    <div className="rounded border bg-primary/50 px-half py-half">
      <div className="flex items-center gap-half">
        <span
          className={`inline-block size-2 shrink-0 rounded-full ${taskStatusDotClass(task.status)}`}
        />
        <span className="text-xs text-normal truncate">{task.name}</span>
        {typeof score === 'number' && (
          <span
            title={
              task.acceptanceVerdict
                ? `Acceptance: ${task.acceptanceVerdict} (${score}/100)`
                : `Acceptance score ${score}/100`
            }
            className={`ml-auto shrink-0 rounded-full px-1.5 py-px text-xs ${scoreBadgeClass(score)}`}
          >
            {Math.round(score)}
          </span>
        )}
      </div>
      {task.branch && (
        <div className="mt-px flex items-center gap-1 text-xs text-low">
          <GitBranchIcon className="size-icon-xs shrink-0" />
          <span className="truncate">{task.branch}</span>
        </div>
      )}
      {(task.terminals ?? []).length > 0 && (
        <div className="mt-px flex gap-1">
          {(task.terminals ?? []).map((term) => (
            <span
              key={term.id}
              title={`${term.role ?? 'T' + String(term.orderIndex + 1)}: ${term.status}`}
              className={`inline-block size-1.5 rounded-full ${terminalStatusDotClass(term.status)}`}
            />
          ))}
        </div>
      )}
      <button
        type="button"
        onClick={() => onViewChanges(workflowId, task.id)}
        className="mt-half flex w-full items-center justify-center gap-1 rounded bg-secondary px-half py-px text-xs text-low hover:text-normal transition-colors"
      >
        <GitDiffIcon className="size-icon-xs" />
        View Changes
      </button>
    </div>
  );
}

export interface RightSidebarProps {
  readonly isCreateMode: boolean;
  readonly isConciergeMode?: boolean;
  readonly conciergeWorkflow?: WorkflowDetailDto | null;
  readonly rightMainPanelMode: RightMainPanelMode | null;
  readonly selectedWorkspace: Workspace | undefined;
  readonly repos: RepoWithTargetBranch[];
}

export function RightSidebar({
  isCreateMode,
  isConciergeMode,
  conciergeWorkflow,
  rightMainPanelMode,
  selectedWorkspace,
  repos,
}: Readonly<RightSidebarProps>) {
  const { selectFile } = useChangesView();
  const { viewProcessInPanel } = useLogsPanel();
  const { diffs } = useWorkspaceContext();
  const { setExpanded } = useExpandedAll();
  const { openTaskChanges } = useOrchestrationDiff();

  if (isConciergeMode) {
    if (!conciergeWorkflow) {
      return (
        <div className="flex h-full flex-col bg-secondary">
          <FeishuChannelContainer />
          <div className="flex flex-1 flex-col items-center justify-center p-base text-center text-sm text-low">
            <p>No active workflow</p>
            <p className="mt-half text-xs">Start a conversation to create a workflow</p>
          </div>
        </div>
      );
    }

    const tasks = conciergeWorkflow.tasks ?? [];
    return (
      <div className="flex h-full flex-col bg-secondary">
        <FeishuChannelContainer />
        <div className="border-b px-base py-half">
          <h3 className="text-sm font-medium text-high truncate">{conciergeWorkflow.name}</h3>
          <div className="flex items-center gap-half mt-px">
            <span className={`rounded-full px-1.5 py-px text-xs ${workflowStatusClass(conciergeWorkflow.status)}`}>
              {conciergeWorkflow.status}
            </span>
            <a
              href={`/pipeline/${conciergeWorkflow.id}`}
              className="ml-auto flex items-center gap-1 text-xs text-brand hover:text-brand/80"
            >
              <ArrowSquareOutIcon className="size-icon-xs" />
              Pipeline
            </a>
          </div>
        </div>
        <div className="flex-1 overflow-y-auto p-base">
          <span className="text-xs font-medium text-low">Tasks ({tasks.length})</span>
          <div className="mt-half flex flex-col gap-half">
            {tasks.map((task) => (
              <ConciergeTaskCard
                key={task.id}
                task={task}
                workflowId={conciergeWorkflow.id}
                onViewChanges={openTaskChanges}
              />
            ))}
          </div>
        </div>
        <div className="border-t px-base py-half">
          <a
            href={`/debug/${conciergeWorkflow.id}`}
            className="flex w-full items-center justify-center gap-1 rounded bg-secondary px-base py-half text-xs text-low hover:text-normal transition-colors"
          >
            Debug Terminals
          </a>
        </div>
      </div>
    );
  }

  if (isCreateMode) {
    return (
      <div className="flex h-full flex-col bg-secondary">
        <FeishuChannelContainer />
        <div className="flex-1 min-h-0 overflow-hidden">
          <GitPanelCreateContainer />
        </div>
      </div>
    );
  }

  if (rightMainPanelMode === RIGHT_MAIN_PANEL_MODES.CHANGES) {
    return (
      <div className="flex flex-col h-full">
        <div className="flex-[7] min-h-0 overflow-hidden">
          <FileTreeContainer
            key={selectedWorkspace?.id}
            workspaceId={selectedWorkspace?.id}
            diffs={diffs}
            onSelectFile={(path) => {
              selectFile(path);
              setExpanded(`diff:${path}`, true);
            }}
          />
        </div>
        <div className="flex-[3] min-h-0 overflow-hidden">
          <GitPanelContainer
            selectedWorkspace={selectedWorkspace}
            repos={repos}
            diffs={diffs}
          />
        </div>
      </div>
    );
  }

  if (rightMainPanelMode === RIGHT_MAIN_PANEL_MODES.LOGS) {
    return (
      <div className="flex flex-col h-full">
        <div className="flex-[7] min-h-0 overflow-hidden">
          <ProcessListContainer />
        </div>
        <div className="flex-[3] min-h-0 overflow-hidden">
          <GitPanelContainer
            selectedWorkspace={selectedWorkspace}
            repos={repos}
            diffs={diffs}
          />
        </div>
      </div>
    );
  }

  if (rightMainPanelMode === RIGHT_MAIN_PANEL_MODES.PREVIEW) {
    return (
      <div className="flex flex-col h-full">
        <div className="flex-[7] min-h-0 overflow-hidden">
          <PreviewControlsContainer
            attemptId={selectedWorkspace?.id}
            onViewProcessInPanel={viewProcessInPanel}
          />
        </div>
        <div className="flex-[3] min-h-0 overflow-hidden">
          <GitPanelContainer
            selectedWorkspace={selectedWorkspace}
            repos={repos}
            diffs={diffs}
          />
        </div>
      </div>
    );
  }

  return (
    <GitPanelContainer
      selectedWorkspace={selectedWorkspace}
      repos={repos}
      diffs={diffs}
    />
  );
}
