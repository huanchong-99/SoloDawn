import { useNavigate } from 'react-router-dom';
import { Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useWorkflows } from '@/hooks/useWorkflows';
import { WorkflowCard } from './WorkflowCard';
import { Button } from '@/components/ui-new/primitives/Button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { Project } from 'shared/types';

interface WorkflowSidebarProps {
  readonly projects: Project[];
  readonly activeProjectId: string;
  readonly onProjectChange: (projectId: string) => void;
  readonly selectedWorkflowId: string | null;
  readonly onSelectWorkflow: (id: string) => void;
}

export function WorkflowSidebar({
  projects,
  activeProjectId,
  onProjectChange,
  selectedWorkflowId,
  onSelectWorkflow,
}: Readonly<WorkflowSidebarProps>) {
  const { t } = useTranslation('workflow');
  const navigate = useNavigate();
  const { data: workflows = [], isLoading } = useWorkflows(activeProjectId);

  return (
    <aside className="w-64 bg-panel border-r border-border p-4 flex flex-col">
      {projects.length > 1 && (
        <div className="mb-3">
          <Select value={activeProjectId} onValueChange={onProjectChange}>
            <SelectTrigger className="w-full text-xs">
              <SelectValue placeholder={t('board.selectProject')}>
                {projects.find((p) => p.id === activeProjectId)?.name ??
                  t('board.selectProject')}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {projects.map((project) => (
                <SelectItem key={project.id} value={project.id}>
                  {project.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
      <div className="text-sm font-semibold mb-3">{t('sidebar.title')}</div>
      {isLoading ? (
        <div className="text-xs text-low">{t('sidebar.loading')}</div>
      ) : (
        <div className="space-y-2 flex-1 min-h-0 overflow-y-auto">
          {workflows.map((workflow) => (
            <WorkflowCard
              key={workflow.id}
              name={workflow.name}
              status={workflow.status}
              selected={selectedWorkflowId === workflow.id}
              onClick={() => onSelectWorkflow(workflow.id)}
            />
          ))}
        </div>
      )}
      <div className="mt-4 pt-4 border-t border-border">
        <Button
          variant="primary"
          size="sm"
          className="w-full"
          onClick={() => navigate('/wizard')}
        >
          <Plus className="w-4 h-4" />
          {t('management.createWorkflow')}
        </Button>
      </div>
    </aside>
  );
}
