import * as React from 'react';
import { create } from 'zustand';
import type { WorkflowDetailDto, WorkflowTaskDto, TerminalDto, TerminalStatus } from 'shared/types';

/**
 * Workflow state management store
 * Manages workflow data, active workflow selection, and real-time updates
 */

interface WorkflowState {
  // State
  workflows: Map<string, WorkflowDetailDto>;
  activeWorkflowId: string | null;
  isLoading: boolean;
  error: string | null;

  // Actions
  setActiveWorkflow: (id: string | null) => void;
  setWorkflows: (workflows: WorkflowDetailDto[]) => void;
  addWorkflow: (workflow: WorkflowDetailDto) => void;
  updateWorkflow: (id: string, updates: Partial<WorkflowDetailDto>) => void;
  removeWorkflow: (id: string) => void;
  updateTaskStatus: (workflowId: string, taskId: string, status: string) => void;
  updateTerminalStatus: (workflowId: string, taskId: string, terminalId: string, status: TerminalStatus) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;

  // Selectors (computed)
  getWorkflow: (id: string) => WorkflowDetailDto | undefined;
  getActiveWorkflow: () => WorkflowDetailDto | undefined;
  getTask: (workflowId: string, taskId: string) => WorkflowTaskDto | undefined;
  getTerminal: (workflowId: string, taskId: string, terminalId: string) => TerminalDto | undefined;
}

const initialState = {
  workflows: new Map<string, WorkflowDetailDto>(),
  activeWorkflowId: null,
  isLoading: false,
  error: null,
};

export const useWorkflowStore = create<WorkflowState>((set, get) => ({
  ...initialState,

  setActiveWorkflow: (id) => {
    set({ activeWorkflowId: id });
  },

  setWorkflows: (workflows) => {
    const workflowMap = new Map<string, WorkflowDetailDto>();
    workflows.forEach((wf) => workflowMap.set(wf.id, wf));
    set({ workflows: workflowMap });
  },

  addWorkflow: (workflow) => {
    set((state) => {
      const newWorkflows = new Map(state.workflows);
      newWorkflows.set(workflow.id, workflow);
      return { workflows: newWorkflows };
    });
  },

  updateWorkflow: (id, updates) => {
    set((state) => {
      const workflow = state.workflows.get(id);
      if (!workflow) return state;

      const newWorkflows = new Map(state.workflows);
      newWorkflows.set(id, { ...workflow, ...updates });
      return { workflows: newWorkflows };
    });
  },

  removeWorkflow: (id) => {
    set((state) => {
      const newWorkflows = new Map(state.workflows);
      newWorkflows.delete(id);

      // Clear active workflow if it was removed
      const newActiveId = state.activeWorkflowId === id ? null : state.activeWorkflowId;

      return { workflows: newWorkflows, activeWorkflowId: newActiveId };
    });
  },

  updateTaskStatus: (workflowId, taskId, status) => {
    set((state) => {
      const workflow = state.workflows.get(workflowId);
      if (!workflow) return state;

      const updatedTasks = workflow.tasks.map((task) =>
        task.id === taskId ? { ...task, status } : task
      );

      const newWorkflows = new Map(state.workflows);
      newWorkflows.set(workflowId, { ...workflow, tasks: updatedTasks });
      return { workflows: newWorkflows };
    });
  },

  updateTerminalStatus: (workflowId, taskId, terminalId, status) => {
    // Helper: Update terminals for a specific task
    const updateTaskTerminals = (task: WorkflowTaskDto): WorkflowTaskDto => {
      if (task.id !== taskId) return task;

      const updatedTerminals = task.terminals.map((terminal) =>
        terminal.id === terminalId ? { ...terminal, status } : terminal
      );

      return { ...task, terminals: updatedTerminals };
    };

    set((state) => {
      const workflow = state.workflows.get(workflowId);
      if (!workflow) return state;

      const updatedTasks = workflow.tasks.map(updateTaskTerminals);

      const newWorkflows = new Map(state.workflows);
      newWorkflows.set(workflowId, { ...workflow, tasks: updatedTasks });
      return { workflows: newWorkflows };
    });
  },

  setLoading: (loading) => {
    set({ isLoading: loading });
  },

  setError: (error) => {
    set({ error });
  },

  reset: () => {
    set(initialState);
  },

  // Selectors
  getWorkflow: (id) => {
    return get().workflows.get(id);
  },

  getActiveWorkflow: () => {
    const { workflows, activeWorkflowId } = get();
    return activeWorkflowId ? workflows.get(activeWorkflowId) : undefined;
  },

  getTask: (workflowId, taskId) => {
    const workflow = get().workflows.get(workflowId);
    return workflow?.tasks.find((t) => t.id === taskId);
  },

  getTerminal: (workflowId, taskId, terminalId) => {
    const task = get().getTask(workflowId, taskId);
    return task?.terminals.find((t) => t.id === terminalId);
  },
}));

/**
 * Hook to get workflow list as array
 * Uses useMemo to prevent unnecessary re-renders
 */
export function useWorkflowList() {
  const workflows = useWorkflowStore((state) => state.workflows);
  return React.useMemo(() => Array.from(workflows.values()), [workflows]);
}

/**
 * Hook to get active workflow
 */
export function useActiveWorkflow() {
  const activeWorkflowId = useWorkflowStore((state) => state.activeWorkflowId);
  const workflows = useWorkflowStore((state) => state.workflows);
  return activeWorkflowId ? workflows.get(activeWorkflowId) : undefined;
}
