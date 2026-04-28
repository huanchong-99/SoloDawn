import type { WorkflowDetailDto, WorkflowTaskDto } from 'shared/types';

const FINAL_REPAIR_TASK_NAME = 'Final Integration Repair';
const FINAL_TASK_STATUSES = new Set(['completed', 'failed', 'cancelled']);

type WorkflowTaskStatusLike = Pick<WorkflowTaskDto, 'name' | 'status'>;

type WorkflowWithTasks = Pick<WorkflowDetailDto, 'status'> & {
  readonly tasks?: readonly WorkflowTaskStatusLike[] | null;
};

export function isRepairingFinalIssues(workflow: WorkflowWithTasks): boolean {
  if (workflow.status !== 'running') {
    return false;
  }

  return (workflow.tasks ?? []).some((task) => {
    const isFinalRepair =
      task.name.trim().toLowerCase() === FINAL_REPAIR_TASK_NAME.toLowerCase();
    return isFinalRepair && !FINAL_TASK_STATUSES.has(task.status);
  });
}

export function getWorkflowDisplayStatus(workflow: WorkflowWithTasks): string {
  return isRepairingFinalIssues(workflow)
    ? 'repairing_final_issues'
    : workflow.status;
}
