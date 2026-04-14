import { useWorkflow } from '@/hooks/useWorkflows';

interface MergeTerminalNodeProps {
  workflowId: string;
}

export function MergeTerminalNode({ workflowId }: Readonly<MergeTerminalNodeProps>) {
  const { data: workflow } = useWorkflow(workflowId);

  if (!workflow) return null;

  // TODO(E10-08): Localize the hardcoded "Merge" label via i18n
  // (`workflow:pipeline.merge` or similar) to match the rest of the pipeline.
  // TODO(E10-09): Consider surfacing merge status (pending/merging/merged/
  // conflict) here instead of a static badge; today the node gives no
  // feedback about the actual merge operation.
  return (
    <div className="flex flex-col gap-2 items-center">
      <div className="text-sm font-semibold">Merge</div>
      <div className="w-32 h-20 rounded border border-border bg-secondary flex items-center justify-center">
        <div className="text-xs">{workflow.targetBranch}</div>
      </div>
    </div>
  );
}
