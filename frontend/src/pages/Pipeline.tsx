import { useParams } from 'react-router-dom';
import { useWorkflow } from '@/hooks/useWorkflows';
import { useWorkflowInvalidation } from '@/hooks/useWorkflowInvalidation';
import { OrchestratorHeader } from '@/components/pipeline/OrchestratorHeader';
import { TaskPipeline } from '@/components/pipeline/TaskPipeline';

export function Pipeline() {
  const { workflowId } = useParams<{ workflowId: string }>();
  const { data: workflow, isLoading } = useWorkflow(workflowId ?? '');

  // Guard: hook internally no-ops when workflowId is undefined
  useWorkflowInvalidation(workflowId);

  if (isLoading) return <div className="p-6 text-low">Loading...</div>;
  if (!workflow) return <div className="p-6 text-low">Workflow not found</div>;

  return (
    <div className="flex h-screen flex-col bg-primary">
      <OrchestratorHeader
        name={workflow.name}
        status={workflow.status}
        model={workflow.orchestratorModel}
      />
      <TaskPipeline workflowId={workflowId ?? ''} />
    </div>
  );
}
