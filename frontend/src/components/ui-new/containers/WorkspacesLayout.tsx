import { useEffect, useState, useCallback, type ReactNode } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Group, Layout, Panel, Separator } from 'react-resizable-panels';
import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
import { ExecutionProcessesProvider } from '@/contexts/ExecutionProcessesContext';
import { CreateModeProvider } from '@/contexts/CreateModeContext';
import { ReviewProvider } from '@/contexts/ReviewProvider';
import { LogsPanelProvider } from '@/contexts/LogsPanelContext';
import { ChangesViewProvider } from '@/contexts/ChangesViewContext';
import { OrchestrationDiffProvider } from '@/contexts/OrchestrationDiffContext';
import { WorkspacesSidebarContainer } from '@/components/ui-new/containers/WorkspacesSidebarContainer';
import { LogsContentContainer } from '@/components/ui-new/containers/LogsContentContainer';
import { WorkspacesMainContainer } from '@/components/ui-new/containers/WorkspacesMainContainer';
import { RightSidebar } from '@/components/ui-new/containers/RightSidebar';
import { ChangesPanelContainer } from '@/components/ui-new/containers/ChangesPanelContainer';
import { CreateChatBoxContainer } from '@/components/ui-new/containers/CreateChatBoxContainer';
import { ConciergeChatContainer } from '@/components/ui-new/containers/ConciergeChatContainer';
import { NavbarContainer } from '@/components/ui-new/containers/NavbarContainer';
import { PreviewBrowserContainer } from '@/components/ui-new/containers/PreviewBrowserContainer';

import {
  PERSIST_KEYS,
  usePaneSize,
  useWorkspacePanelState,
  RIGHT_MAIN_PANEL_MODES,
} from '@/stores/useUiPreferencesStore';

import { useConciergeSession } from '@/hooks/useConcierge';
import { useWorkflow } from '@/hooks/useWorkflows';
import {
  useWorkflowEvents,
  type TerminalCompletedPayload,
  type AcceptanceReviewPayload,
  type QualityGateResultPayload,
} from '@/stores/wsStore';
import type { OrchestrationDiffTarget } from '@/contexts/OrchestrationDiffContext';

interface ModeProviderProps {
  isCreateMode: boolean;
  executionProps: {
    key: string;
    attemptId?: string;
    sessionId?: string;
  };
  children: ReactNode;
}

function ModeProvider({
  isCreateMode,
  executionProps,
  children,
}: Readonly<ModeProviderProps>) {
  if (isCreateMode) {
    return <CreateModeProvider>{children}</CreateModeProvider>;
  }
  return (
    <ExecutionProcessesProvider
      key={executionProps.key}
      attemptId={executionProps.attemptId}
      sessionId={executionProps.sessionId}
    >
      {children}
    </ExecutionProcessesProvider>
  );
}

export function WorkspacesLayout() {
  const {
    workspaceId,
    workspace: selectedWorkspace,
    isLoading,
    isCreateMode,
    selectedSession,
    selectedSessionId,
    sessions,
    selectSession,
    repos,
    isNewSessionMode,
    startNewSession,
  } = useWorkspaceContext();

  const [searchParams] = useSearchParams();
  const conciergeSessionId = searchParams.get('conciergeId');
  const isConciergeMode = Boolean(conciergeSessionId);

  const { data: conciergeSession } = useConciergeSession(conciergeSessionId);
  const conciergeWorkflowId = conciergeSession?.activeWorkflowId ?? null;
  const { data: conciergeWorkflow } = useWorkflow(conciergeWorkflowId ?? '');

  // FE-2: In concierge mode there is no `:workspaceId`, so the workspace-scoped
  // panel state (`setRightMainPanelMode` early-returns on a falsy key) cannot be
  // driven and the panel default stays `null` (closed). Derive a stable synthetic
  // key from the active concierge workflow so auto-open actually takes effect.
  const panelStateKey = (() => {
    if (isCreateMode) return undefined;
    if (isConciergeMode) {
      return conciergeWorkflowId
        ? `concierge:${conciergeWorkflowId}`
        : `concierge:${conciergeSessionId}`;
    }
    return workspaceId;
  })();

  // Use workspace-specific panel state (pass undefined when in create mode)
  const {
    isLeftSidebarVisible,
    isLeftMainPanelVisible,
    isRightSidebarVisible,
    rightMainPanelMode,
    setRightMainPanelMode,
    setLeftSidebarVisible,
    setLeftMainPanelVisible,
  } = useWorkspacePanelState(panelStateKey);

  // FE-2 (Phase C step 13): auto-surface the per-task Changes/Audit panel in the
  // orchestration workspace. Mirrors the legacy `useWorkflowEvents` wiring on
  // Board/Workflows pages, but instead of only badging it (a) records the task
  // as the active diff target and (b) calls `setRightMainPanelMode(CHANGES)` to
  // AUTO-OPEN — without this the produced changes stay invisible (Q2 Break 6).
  const [orchestrationDiffTarget, setOrchestrationDiffTarget] =
    useState<OrchestrationDiffTarget | null>(null);

  const openTaskChanges = useCallback(
    (wfId: string, taskId: string) => {
      setOrchestrationDiffTarget({ workflowId: wfId, taskId });
      setRightMainPanelMode(RIGHT_MAIN_PANEL_MODES.CHANGES);
    },
    [setRightMainPanelMode]
  );

  const handleTerminalCompleted = useCallback(
    (payload: TerminalCompletedPayload) => {
      if (conciergeWorkflowId && payload.taskId) {
        openTaskChanges(conciergeWorkflowId, payload.taskId);
      }
    },
    [conciergeWorkflowId, openTaskChanges]
  );

  const handleAcceptanceReview = useCallback(
    (payload: AcceptanceReviewPayload) => {
      if (conciergeWorkflowId && payload.taskId) {
        openTaskChanges(conciergeWorkflowId, payload.taskId);
      }
    },
    [conciergeWorkflowId, openTaskChanges]
  );

  const handleQualityGateResult = useCallback(
    (payload: QualityGateResultPayload) => {
      if (conciergeWorkflowId && payload.taskId) {
        openTaskChanges(conciergeWorkflowId, payload.taskId);
      }
    },
    [conciergeWorkflowId, openTaskChanges]
  );

  useWorkflowEvents(isConciergeMode ? conciergeWorkflowId : null, {
    onTerminalCompleted: handleTerminalCompleted,
    onAcceptanceReviewResult: handleAcceptanceReview,
    onQualityGateResult: handleQualityGateResult,
  });

  const taskDiffSource =
    isConciergeMode && orchestrationDiffTarget
      ? {
          workflowId: orchestrationDiffTarget.workflowId,
          taskId: orchestrationDiffTarget.taskId,
        }
      : undefined;

  // Ensure left panels visible when right main panel hidden
  useEffect(() => {
    if (rightMainPanelMode === null) {
      setLeftSidebarVisible(true);
      if (!isLeftMainPanelVisible) setLeftMainPanelVisible(true);
    }
  }, [
    isLeftMainPanelVisible,
    rightMainPanelMode,
    setLeftSidebarVisible,
    setLeftMainPanelVisible,
  ]);

  const [rightMainPanelSize, setRightMainPanelSize] = usePaneSize(
    PERSIST_KEYS.rightMainPanel,
    50
  );

  const defaultLayout: Layout =
    typeof rightMainPanelSize === 'number'
      ? {
          'left-main': 100 - rightMainPanelSize,
          'right-main': rightMainPanelSize,
        }
      : { 'left-main': 50, 'right-main': 50 };

  const onLayoutChange = (layout: Layout) => {
    if (rightMainPanelMode !== null)
      setRightMainPanelSize(layout['right-main']);
  };

  return (
    <ModeProvider
      isCreateMode={isCreateMode}
      executionProps={{
        key: `${selectedWorkspace?.id}-${selectedSessionId}`,
        attemptId: selectedWorkspace?.id,
        sessionId: selectedSessionId,
      }}
    >
      <div className="flex flex-col h-full">
        <NavbarContainer />
        <div className="flex flex-1 min-h-0">
          {isLeftSidebarVisible && (
            <div className="w-[300px] max-w-[30vw] shrink-0 h-full overflow-hidden">
              <WorkspacesSidebarContainer />
            </div>
          )}

          <div className="flex-1 min-w-0 h-full">
            <ReviewProvider attemptId={selectedWorkspace?.id}>
              <LogsPanelProvider>
                <ChangesViewProvider>
                 <OrchestrationDiffProvider
                  isOrchestration={isConciergeMode}
                  diffTarget={orchestrationDiffTarget}
                  openTaskChanges={openTaskChanges}
                 >
                  <div className="flex h-full">
                    <Group
                      orientation="horizontal"
                      className="flex-1 min-w-0 h-full"
                      defaultLayout={defaultLayout}
                      onLayoutChange={onLayoutChange}
                    >
                      {isLeftMainPanelVisible && (
                        <Panel
                          id="left-main"
                          minSize={20}
                          className="min-w-0 h-full overflow-hidden"
                        >
                          {(() => {
                            if (isConciergeMode) {
                              return (
                                <ConciergeChatContainer
                                  initialSessionId={conciergeSessionId}
                                />
                              );
                            }
                            if (isCreateMode) {
                              return <CreateChatBoxContainer />;
                            }
                            return (
                              <WorkspacesMainContainer
                                selectedWorkspace={selectedWorkspace ?? null}
                                selectedSession={selectedSession}
                                sessions={sessions}
                                onSelectSession={selectSession}
                                isLoading={isLoading}
                                isNewSessionMode={isNewSessionMode}
                                onStartNewSession={startNewSession}
                              />
                            );
                          })()}
                        </Panel>
                      )}

                      {isLeftMainPanelVisible &&
                        rightMainPanelMode !== null && (
                          <Separator
                            id="main-separator"
                            className="w-1 bg-transparent hover:bg-brand/50 transition-colors cursor-col-resize"
                          />
                        )}

                      {rightMainPanelMode !== null && (
                        <Panel
                          id="right-main"
                          minSize={20}
                          className="min-w-0 h-full overflow-hidden"
                        >
                          {rightMainPanelMode ===
                            RIGHT_MAIN_PANEL_MODES.CHANGES && (
                            <ChangesPanelContainer
                              attemptId={selectedWorkspace?.id}
                              taskDiffSource={taskDiffSource}
                            />
                          )}
                          {rightMainPanelMode ===
                            RIGHT_MAIN_PANEL_MODES.LOGS && (
                            <LogsContentContainer />
                          )}
                          {rightMainPanelMode ===
                            RIGHT_MAIN_PANEL_MODES.PREVIEW && (
                            <PreviewBrowserContainer
                              attemptId={selectedWorkspace?.id}
                            />
                          )}
                        </Panel>
                      )}
                    </Group>

                    {isRightSidebarVisible && (
                      <div className="w-[300px] max-w-[30vw] shrink-0 h-full overflow-hidden">
                        <RightSidebar
                          isCreateMode={isCreateMode}
                          isConciergeMode={isConciergeMode}
                          conciergeWorkflow={conciergeWorkflow ?? null}
                          rightMainPanelMode={rightMainPanelMode}
                          selectedWorkspace={selectedWorkspace}
                          repos={repos}
                        />
                      </div>
                    )}
                  </div>
                 </OrchestrationDiffProvider>
                </ChangesViewProvider>
              </LogsPanelProvider>
            </ReviewProvider>
          </div>
        </div>
      </div>
    </ModeProvider>
  );
}
