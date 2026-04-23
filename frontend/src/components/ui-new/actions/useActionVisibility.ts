import { useMemo } from 'react';
import {
  useUiPreferencesStore,
  useWorkspacePanelState,
} from '@/stores/useUiPreferencesStore';
import { useDiffViewStore, useDiffViewMode } from '@/stores/useDiffViewStore';
import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
import { useUserSystem } from '@/components/ConfigProvider';
import { useDevServer } from '@/hooks/useDevServer';
import { useBranchStatus } from '@/hooks/useBranchStatus';
import { useExecutionProcessesContext } from '@/contexts/ExecutionProcessesContext';
import type { Workspace, Merge } from 'shared/types';
import type {
  ActionVisibilityContext,
  ActionDefinition,
  ActionIcon,
  DevServerState,
  NavbarItem,
  ContextBarItem,
} from './index';
import { resolveLabel } from './index';
import type { CommandBarPage } from './pages';

/**
 * Hook that builds the visibility context from stores/context.
 * Used by both NavbarContainer and CommandBarDialog to evaluate
 * action visibility and state conditions.
 */
export function useActionVisibilityContext(): ActionVisibilityContext {
  const { workspace, workspaceId, isCreateMode, repos } = useWorkspaceContext();
  // Use workspace-specific panel state (pass undefined when in create mode)
  const panelState = useWorkspacePanelState(
    isCreateMode ? undefined : workspaceId
  );
  const diffPaths = useDiffViewStore((s) => s.diffPaths);
  const diffViewMode = useDiffViewMode();
  const expanded = useUiPreferencesStore((s) => s.expanded);
  const { config } = useUserSystem();
  const { isStarting, isStopping, runningDevServers } =
    useDevServer(workspaceId);
  const { data: branchStatus } = useBranchStatus(workspaceId);
  const { isAttemptRunningVisible } = useExecutionProcessesContext();

  return useMemo(() => {
    // Compute isAllDiffsExpanded
    const diffKeys = diffPaths.map((p) => `diff:${p}`);
    const isAllDiffsExpanded =
      diffKeys.length > 0 && diffKeys.every((k) => expanded[k] !== false);

    // Compute dev server state
    let devServerState: DevServerState;
    if (isStarting) {
      devServerState = 'starting';
    } else if (isStopping) {
      devServerState = 'stopping';
    } else if (runningDevServers.length > 0) {
      devServerState = 'running';
    } else {
      devServerState = 'stopped';
    }

    // Compute git state from branch status
    const hasOpenPR =
      branchStatus?.some((repo) =>
        repo.merges?.some(
          (m: Merge) => m.type === 'pr' && m.prInfo.status === 'open'
        )
      ) ?? false;

    const hasUnpushedCommits =
      branchStatus?.some((repo) => (repo.remote_commits_ahead ?? 0) > 0) ??
      false;

    return {
      rightMainPanelMode: panelState.rightMainPanelMode,
      isLeftSidebarVisible: panelState.isLeftSidebarVisible,
      isLeftMainPanelVisible: panelState.isLeftMainPanelVisible,
      isRightSidebarVisible: panelState.isRightSidebarVisible,
      isCreateMode,
      hasWorkspace: !!workspace,
      workspaceArchived: workspace?.archived ?? false,
      hasDiffs: diffPaths.length > 0,
      diffViewMode,
      isAllDiffsExpanded,
      editorType: config?.editor?.editorType ?? null,
      devServerState,
      runningDevServers,
      hasGitRepos: repos.length > 0,
      hasMultipleRepos: repos.length > 1,
      hasOpenPR,
      hasUnpushedCommits,
      isAttemptRunning: isAttemptRunningVisible,
    };
  }, [
    panelState.rightMainPanelMode,
    panelState.isLeftSidebarVisible,
    panelState.isLeftMainPanelVisible,
    panelState.isRightSidebarVisible,
    isCreateMode,
    workspace,
    repos,
    diffPaths,
    diffViewMode,
    expanded,
    config?.editor?.editorType,
    isStarting,
    isStopping,
    runningDevServers,
    branchStatus,
    isAttemptRunningVisible,
  ]);
}

/**
 * Helper to check if an action is visible given the current context.
 * If the action has no isVisible condition, it's always visible.
 */
export function isActionVisible(
  action: ActionDefinition,
  ctx: ActionVisibilityContext
): boolean {
  return action.isVisible?.(ctx) ?? true;
}

function isDividerItem(
  item: NavbarItem | ContextBarItem
): item is { readonly type: 'divider' } {
  return 'type' in item && item.type === 'divider';
}

/**
 * Filter action items by visibility while normalizing divider placement:
 * - keeps dividers in the middle
 * - removes leading/trailing dividers
 * - removes consecutive dividers
 */
export function filterVisibleActionItems<T extends NavbarItem | ContextBarItem>(
  items: readonly T[],
  ctx: ActionVisibilityContext
): T[] {
  const filtered = items.filter((item) => {
    if (isDividerItem(item)) return true;
    return isActionVisible(item, ctx);
  });

  const result: T[] = [];
  for (const item of filtered) {
    if (isDividerItem(item)) {
      const lastItem = result.at(-1);
      if (result.length > 0 && lastItem && !isDividerItem(lastItem)) {
        result.push(item);
      }
    } else {
      result.push(item);
    }
  }

  const lastItem = result.at(-1);
  if (result.length > 0 && lastItem && isDividerItem(lastItem)) {
    result.pop();
  }

  return result;
}

/**
 * Helper to check if a page is visible given the current context.
 * If the page has no isVisible condition, it's always visible.
 */
export function isPageVisible(
  page: CommandBarPage,
  ctx: ActionVisibilityContext
): boolean {
  return page.isVisible?.(ctx) ?? true;
}

/**
 * Helper to check if an action is active given the current context.
 * If the action has no isActive callback, returns false.
 */
export function isActionActive(
  action: ActionDefinition,
  ctx: ActionVisibilityContext
): boolean {
  return action.isActive?.(ctx) ?? false;
}

/**
 * Helper to check if an action is enabled given the current context.
 * If the action has no isEnabled callback, returns true (enabled by default).
 */
export function isActionEnabled(
  action: ActionDefinition,
  ctx: ActionVisibilityContext
): boolean {
  return action.isEnabled?.(ctx) ?? true;
}

/**
 * Get the icon for an action, considering dynamic icon callbacks.
 * Falls back to the static icon property.
 */
export function getActionIcon(
  action: ActionDefinition,
  ctx: ActionVisibilityContext
): ActionIcon {
  return action.getIcon ? action.getIcon(ctx) : action.icon;
}

/**
 * Get the tooltip for an action, considering dynamic tooltip callbacks.
 * Falls back to the resolved label.
 */
export function getActionTooltip(
  action: ActionDefinition,
  ctx: ActionVisibilityContext
): string {
  return action.getTooltip ? action.getTooltip(ctx) : resolveLabel(action);
}

/**
 * Get the label for an action, considering dynamic label callbacks.
 * Falls back to the resolved static label.
 */
export function getActionLabel(
  action: ActionDefinition,
  ctx: ActionVisibilityContext,
  workspace?: Workspace
): string {
  return action.getLabel
    ? action.getLabel(ctx)
    : resolveLabel(action, workspace);
}
