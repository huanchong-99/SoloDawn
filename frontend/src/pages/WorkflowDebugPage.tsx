import { useCallback, useMemo } from 'react';
import { useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { useWorkflow, workflowKeys } from '@/hooks/useWorkflows';
import { useWorkflowEvents } from '@/stores/wsStore';
import { TerminalDebugView } from '@/components/terminal/TerminalDebugView';
import type { Terminal, TerminalStatus } from '@/components/workflow/TerminalCard';
import type { WorkflowTask } from '@/components/workflow/PipelineView';

/**
 * Maps backend terminal status string to frontend TerminalStatus type
 */
function mapTerminalStatus(status: string): TerminalStatus {
  switch (status) {
    case 'idle':
    case 'not_started':
      return 'not_started';
    case 'starting':
      return 'starting';
    case 'waiting':
      return 'waiting';
    case 'running':
    case 'working':
      return 'working';
    case 'completed':
      return 'completed';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    case 'review_passed':
      return 'review_passed';
    case 'review_rejected':
      return 'review_rejected';
    default:
      return 'not_started';
  }
}

export function WorkflowDebugPage() {
  const { workflowId } = useParams<{ workflowId: string }>();
  const queryClient = useQueryClient();
  // G28-003: Use WebSocket events instead of 1.5s polling for real-time updates
  const { data: workflow, isLoading } = useWorkflow(workflowId ?? '');

  const invalidateWorkflow = useCallback(() => {
    if (!workflowId) return;
    queryClient.invalidateQueries({ queryKey: workflowKeys.byId(workflowId) });
  }, [queryClient, workflowId]);

  const workflowEventHandlers = useMemo(
    () => ({
      onWorkflowStatusChanged: invalidateWorkflow,
      onTaskStatusChanged: invalidateWorkflow,
      onTerminalStatusChanged: invalidateWorkflow,
      onTerminalCompleted: invalidateWorkflow,
      onGitCommitDetected: invalidateWorkflow,
    }),
    [invalidateWorkflow]
  );

  useWorkflowEvents(workflowId, workflowEventHandlers);

  const workflowTasks = workflow?.tasks ?? [];

  if (isLoading) return <div className="p-6 text-low">Loading...</div>;
  if (!workflow) return <div className="p-6 text-low">Workflow not found</div>;

  // Construct WebSocket URL for PTY connection
  // TerminalEmulator appends `/terminal/${terminalId}` to this base URL
  const wsProtocol = globalThis.location.protocol === 'https:' ? 'wss' : 'ws';
  const wsUrl = `${wsProtocol}://${globalThis.location.host}/api`;

  // Map WorkflowTaskDto to WorkflowTask with Terminal type conversion
  const tasks: (WorkflowTask & { terminals: Terminal[] })[] = workflowTasks.map(
    (taskDto) => ({
      id: taskDto.id,
      name: taskDto.name,
      branch: taskDto.branch,
      terminals: (taskDto.terminals ?? []).map(
        (termDto): Terminal => ({
          id: termDto.id,
          workflowTaskId: termDto.workflowTaskId,
          cliTypeId: termDto.cliTypeId,
          modelConfigId: termDto.modelConfigId ?? undefined,
          role: termDto.role ?? undefined,
          orderIndex: termDto.orderIndex,
          status: mapTerminalStatus(termDto.status),
        })
      ),
    })
  );

  return (
    <div className="flex h-screen bg-primary">
      <TerminalDebugView tasks={tasks} wsUrl={wsUrl} />
    </div>
  );
}
