import { useCallback } from 'react';
import { useParams } from 'react-router-dom';
import {
  getWorkflowActions,
  useStartWorkflow,
  useWorkflow,
  type WorkflowStatusEnum,
} from '@/hooks/useWorkflows';
import { useWorkflowInvalidation } from '@/hooks/useWorkflowInvalidation';
import { OrchestratorHeader } from '@/components/pipeline/OrchestratorHeader';
import { TaskPipeline } from '@/components/pipeline/TaskPipeline';

export function Pipeline() {
  const { workflowId } = useParams<{ workflowId: string }>();
  const { data: workflow, isLoading } = useWorkflow(workflowId ?? '');

  // Guard: hook internally no-ops when workflowId is undefined
  useWorkflowInvalidation(workflowId);

  // Resume goes through POST /api/workflows/{id}/start: that endpoint accepts
  // `paused` and CASes it back to `ready` before re-driving the runtime, and it
  // is the only start path that also covers DIY (non-orchestrator) workflows.
  const { mutate: startWorkflow, isPending: isResuming } = useStartWorkflow();
  const handleResume = useCallback(() => {
    if (!workflowId) return;
    startWorkflow({ workflow_id: workflowId });
  }, [startWorkflow, workflowId]);

  if (isLoading) return <div className="p-6 text-low">Loading...</div>;
  if (!workflow) return <div className="p-6 text-low">Workflow not found</div>;

  const actions = getWorkflowActions(workflow.status as WorkflowStatusEnum);

  return (
    <div className="flex h-screen flex-col bg-primary">
      <OrchestratorHeader
        name={workflow.name}
        status={workflow.status}
        model={workflow.orchestratorModel}
        canResume={actions.canResume}
        isResuming={isResuming}
        onResume={handleResume}
      />
      <TaskPipeline workflowId={workflowId ?? ''} />
    </div>
  );
}
