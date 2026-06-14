import React, { createContext, useContext, useMemo } from 'react';

/**
 * FE-2 (Phase C): bridge between the orchestration chat / right-sidebar
 * affordances (deep in the left/right panels) and `WorkspacesLayout`, which
 * owns the right-main panel mode and renders `ChangesPanelContainer`.
 *
 * In concierge / orchestration mode there is no `Workspace`/`:workspaceId`
 * (see Q2 spec Break 4/6), so the diff target is a `{ workflowId, taskId }`
 * pair (consumed by `useWorkflowTaskDiff` → the Phase-A branch-diff endpoint).
 * `openTaskChanges` both sets that target and auto-opens the CHANGES panel.
 *
 * State lives in `WorkspacesLayout`; this context is a thin pass-through so the
 * single source of truth (the WS-event auto-open and the manual buttons) stay
 * in sync.
 */
export interface OrchestrationDiffTarget {
  workflowId: string;
  taskId: string;
}

interface OrchestrationDiffContextValue {
  /** Currently selected per-task diff target (null when none selected). */
  diffTarget: OrchestrationDiffTarget | null;
  /** Select a task as the diff target AND auto-open the CHANGES panel. */
  openTaskChanges: (workflowId: string, taskId: string) => void;
  /** Whether the surrounding view is in orchestration / concierge mode. */
  isOrchestration: boolean;
}

const defaultValue: OrchestrationDiffContextValue = {
  diffTarget: null,
  openTaskChanges: () => {},
  isOrchestration: false,
};

const OrchestrationDiffContext =
  createContext<OrchestrationDiffContextValue>(defaultValue);

interface OrchestrationDiffProviderProps {
  readonly isOrchestration: boolean;
  readonly diffTarget: OrchestrationDiffTarget | null;
  readonly openTaskChanges: (workflowId: string, taskId: string) => void;
  readonly children: React.ReactNode;
}

export function OrchestrationDiffProvider({
  isOrchestration,
  diffTarget,
  openTaskChanges,
  children,
}: OrchestrationDiffProviderProps) {
  const value = useMemo(
    () => ({ diffTarget, openTaskChanges, isOrchestration }),
    [diffTarget, openTaskChanges, isOrchestration]
  );

  return (
    <OrchestrationDiffContext.Provider value={value}>
      {children}
    </OrchestrationDiffContext.Provider>
  );
}

export function useOrchestrationDiff(): OrchestrationDiffContextValue {
  return useContext(OrchestrationDiffContext);
}
