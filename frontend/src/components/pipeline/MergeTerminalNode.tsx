import { useTranslation } from 'react-i18next';

import { useWorkflow } from '@/hooks/useWorkflows';

interface MergeTerminalNodeProps {
  workflowId: string;
}

export function MergeTerminalNode({ workflowId }: Readonly<MergeTerminalNodeProps>) {
  const { t } = useTranslation('workflow');
  const { data: workflow } = useWorkflow(workflowId);

  if (!workflow) return null;

  // NOTE(E10-09): merge status surfacing is a product/UX decision and tracked
  // separately from the i18n fix (E10-08). Static badge kept for now.
  return (
    <div className="flex flex-col gap-2 items-center">
      <div className="text-sm font-semibold">{t('pipeline.merge')}</div>
      <div className="w-32 h-20 rounded border border-border bg-secondary flex items-center justify-center">
        <div className="text-xs">{workflow.targetBranch}</div>
      </div>
    </div>
  );
}
