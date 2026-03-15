import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { WorkflowSidebar } from '@/components/board/WorkflowSidebar';
import { WorkflowKanbanBoard } from '@/components/board/WorkflowKanbanBoard';
import { TerminalActivityPanel } from '@/components/board/TerminalActivityPanel';
import { StatusBar } from '@/components/board/StatusBar';
import { ViewHeader } from '@/components/ui-new/primitives/ViewHeader';
import { useWorkflowEvents } from '@/stores/wsStore';
import { useQueryClient } from '@tanstack/react-query';
import { workflowKeys } from '@/hooks/useWorkflows';
import { qualityKeys } from '@/hooks/useQualityGate';
import { useSearchParams } from 'react-router-dom';
import { useProjects } from '@/hooks/useProjects';

export function Board() {
  const { t } = useTranslation('workflow');
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();

  const { projects } = useProjects();

  const projectIdFromUrl = searchParams.get('projectId');
  const validProjectId =
    projectIdFromUrl && projects.some((p) => p.id === projectIdFromUrl)
      ? projectIdFromUrl
      : projects[0]?.id ?? '';

  useEffect(() => {
    if (projects.length > 0 && projectIdFromUrl !== validProjectId) {
      const next = new URLSearchParams(searchParams);
      next.set('projectId', validProjectId);
      setSearchParams(next, { replace: true });
    }
  }, [projectIdFromUrl, validProjectId, projects.length, searchParams, setSearchParams]);

  const handleProjectChange = useCallback(
    (newProjectId: string) => {
      const next = new URLSearchParams(searchParams);
      next.set('projectId', newProjectId);
      next.delete('workflowId');
      setSearchParams(next, { replace: true });
      setSelectedWorkflowId(null);
    },
    [searchParams, setSearchParams]
  );

  const workflowIdFromUrl = searchParams.get('workflowId');
  const [selectedWorkflowId, setSelectedWorkflowId] = useState<string | null>(
    workflowIdFromUrl
  );

  useEffect(() => {
    if (selectedWorkflowId === workflowIdFromUrl) {
      return;
    }

    setSearchParams((prev) => {
      const nextSearchParams = new URLSearchParams(prev);
      if (selectedWorkflowId) {
        nextSearchParams.set('workflowId', selectedWorkflowId);
      } else {
        nextSearchParams.delete('workflowId');
      }
      return nextSearchParams;
    }, { replace: true });
  }, [
    selectedWorkflowId,
    setSearchParams,
    workflowIdFromUrl,
  ]);

  // G29-008: Debounced WS invalidation to avoid rapid-fire cache busting
  // TODO: G11-006 — Consider batching multiple rapid status changes into a single
  // invalidation cycle to further reduce React Query refetch storms.
  const invalidationTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleRealtimeWorkflowSignal = useCallback(() => {
    if (!selectedWorkflowId) return;

    if (invalidationTimerRef.current) {
      clearTimeout(invalidationTimerRef.current);
    }

    invalidationTimerRef.current = setTimeout(() => {
      // G11-002 / G29-003: Invalidate both byId AND forProject list
      queryClient.invalidateQueries({
        queryKey: workflowKeys.byId(selectedWorkflowId),
      });
      queryClient.invalidateQueries({
        queryKey: workflowKeys.forProject(validProjectId),
      });
      invalidationTimerRef.current = null;
    }, 300);
  }, [queryClient, selectedWorkflowId, validProjectId]);

  // Cleanup debounce timer on unmount
  useEffect(() => {
    return () => {
      if (invalidationTimerRef.current) {
        clearTimeout(invalidationTimerRef.current);
      }
    };
  }, []);

  const handleQualityGateResult = useCallback((payload: unknown) => {
    const data = payload as { workflowId?: string; terminalId?: string; runId?: string };
    if (data.terminalId) {
      queryClient.invalidateQueries({
        queryKey: qualityKeys.latestForTerminal(data.terminalId),
      });
    }
    if (data.workflowId) {
      queryClient.invalidateQueries({
        queryKey: qualityKeys.runsForWorkflow(data.workflowId),
      });
    }
    // G31-007: Also invalidate runDetail and issuesForRun when runId is available
    if (data.runId) {
      queryClient.invalidateQueries({
        queryKey: qualityKeys.runDetail(data.runId),
      });
      queryClient.invalidateQueries({
        queryKey: qualityKeys.issuesForRun(data.runId),
      });
    }
  }, [queryClient]);

  // G29-004: Prompt events trigger invalidation so UI reflects prompt state
  const handlePromptEvent = useCallback(() => {
    handleRealtimeWorkflowSignal();
  }, [handleRealtimeWorkflowSignal]);

  // G08-006: When the server reports a system.lagged event (messages were dropped),
  // invalidate all workflow caches to resync state.
  const handleSystemLagged = useCallback(() => {
    console.warn('[Board] system.lagged received — invalidating all workflow caches');
    queryClient.invalidateQueries({ queryKey: workflowKeys.all });
  }, [queryClient]);

  const workflowEventHandlers = useMemo(
    () => ({
      onWorkflowStatusChanged: handleRealtimeWorkflowSignal,
      onTaskStatusChanged: handleRealtimeWorkflowSignal,
      onTerminalStatusChanged: handleRealtimeWorkflowSignal,
      onTerminalCompleted: handleRealtimeWorkflowSignal,
      onGitCommitDetected: handleRealtimeWorkflowSignal,
      onTerminalPromptDetected: handlePromptEvent,
      onTerminalPromptDecision: handlePromptEvent,
      onQualityGateResult: handleQualityGateResult,
      // G08-006: Invalidate all caches when messages were dropped
      onSystemLagged: handleSystemLagged,
    }),
    [handleRealtimeWorkflowSignal, handlePromptEvent, handleQualityGateResult, handleSystemLagged]
  );

  useWorkflowEvents(selectedWorkflowId, workflowEventHandlers);

  return (
    <div className="app-canvas flex h-full min-h-0 w-full">
      <WorkflowSidebar
        projects={projects}
        activeProjectId={validProjectId}
        onProjectChange={handleProjectChange}
        selectedWorkflowId={selectedWorkflowId}
        onSelectWorkflow={setSelectedWorkflowId}
      />
      <main className="flex min-w-0 min-h-0 flex-1 flex-col overflow-hidden">
        <div className="flex min-h-0 flex-1 flex-col gap-4 px-6 pt-6 overflow-hidden">
          <ViewHeader
            eyebrow={t('board.eyebrow', { defaultValue: 'Kanban' })}
            title={t('board.title', { defaultValue: 'Workflow board' })}
            description={t('board.description', {
              defaultValue: 'Drag tasks between columns to track progress.',
            })}
          />
          <div className="min-h-0 flex-1 overflow-auto">
            <WorkflowKanbanBoard workflowId={selectedWorkflowId} />
          </div>
        </div>
        <TerminalActivityPanel workflowId={selectedWorkflowId} />
        <StatusBar workflowId={selectedWorkflowId} />
      </main>
    </div>
  );
}
