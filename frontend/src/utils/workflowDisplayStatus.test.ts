import { describe, expect, it } from 'vitest';
import {
  getWorkflowDisplayStatus,
  isRepairingFinalIssues,
} from './workflowDisplayStatus';

describe('workflowDisplayStatus', () => {
  it('shows running workflows with an active final repair task as repairing final issues', () => {
    const workflow = {
      status: 'running',
      tasks: [
        {
          name: 'Final Integration Repair',
          status: 'running',
        },
      ],
    };

    expect(isRepairingFinalIssues(workflow)).toBe(true);
    expect(getWorkflowDisplayStatus(workflow)).toBe('repairing_final_issues');
  });

  it('keeps the real status when final repair has finished or workflow is not running', () => {
    expect(
      getWorkflowDisplayStatus({
        status: 'running',
        tasks: [{ name: 'Final Integration Repair', status: 'completed' }],
      })
    ).toBe('running');

    expect(
      getWorkflowDisplayStatus({
        status: 'completed',
        tasks: [{ name: 'Final Integration Repair', status: 'running' }],
      })
    ).toBe('completed');
  });
});
