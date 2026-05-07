import type { RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { CaretRightIcon } from '@phosphor-icons/react';
import type { Session } from 'shared/types';
import type { WorkspaceWithSession } from '@/types/attempt';
import { SessionChatBoxContainer } from '@/components/ui-new/containers/SessionChatBoxContainer';

interface PlanningMessage {
  readonly id: string;
  readonly role: 'user' | 'assistant';
  readonly content: string;
}
import { ContextBarContainer } from '@/components/ui-new/containers/ContextBarContainer';
import { ConversationList } from '../containers/ConversationListContainer';
import { EntriesProvider } from '@/contexts/EntriesContext';
import { MessageEditProvider } from '@/contexts/MessageEditContext';
import { RetryUiProvider } from '@/contexts/RetryUiContext';
import { ApprovalFeedbackProvider } from '@/contexts/ApprovalFeedbackContext';

interface DiffStats {
  filesChanged: number;
  linesAdded: number;
  linesRemoved: number;
}

interface WorkspacesMainProps {
  workspaceWithSession: WorkspaceWithSession | undefined;
  sessions: Session[];
  onSelectSession: (sessionId: string) => void;
  isLoading: boolean;
  containerRef: RefObject<HTMLElement | null>;
  projectId?: string;
  /** Whether user is creating a new session */
  isNewSessionMode?: boolean;
  /** Callback to start new session mode */
  onStartNewSession?: () => void;
  /** Diff statistics from the workspace */
  diffStats?: DiffStats;
  /** Planning draft conversation messages */
  planningMessages?: readonly PlanningMessage[];
  /** Whether planning messages are expanded */
  showPlanningMessages?: boolean;
  /** Toggle planning messages visibility */
  onTogglePlanningMessages?: () => void;
}

export function WorkspacesMain({
  workspaceWithSession,
  sessions,
  onSelectSession,
  isLoading,
  containerRef,
  projectId,
  isNewSessionMode,
  onStartNewSession,
  diffStats,
  planningMessages,
  showPlanningMessages = true,
  onTogglePlanningMessages,
}: Readonly<WorkspacesMainProps>) {
  const { t } = useTranslation(['tasks', 'common']);
  const session = workspaceWithSession?.session;

  // Always render the main structure to prevent chat box flash during workspace transitions
  return (
    <main
      ref={containerRef as React.RefObject<HTMLElement>}
      className="relative flex flex-1 flex-col bg-primary h-full"
    >
      <ApprovalFeedbackProvider>
        <EntriesProvider
          key={
            workspaceWithSession
              ? `${workspaceWithSession.id}-${session?.id}`
              : 'empty'
          }
        >
          {/* Conversation content - conditional based on loading/workspace state */}
          <MessageEditProvider>
            {(() => {
              if (isLoading) {
                return (
                  <div className="flex-1 flex items-center justify-center">
                    <p className="text-low">{t('common:workspaces.loading')}</p>
                  </div>
                );
              }
              if (!workspaceWithSession) {
                return (
                  <div className="flex-1 flex items-center justify-center">
                    <p className="text-low">
                      {t('common:workspaces.selectToStart')}
                    </p>
                  </div>
                );
              }
              return (
                <div className="flex-1 min-h-0 overflow-y-auto flex justify-center">
                  <div className="w-chat max-w-full h-full">
                    {planningMessages && planningMessages.length > 0 && (
                      <div className="border-b border-primary mb-base">
                        <button
                          onClick={() => onTogglePlanningMessages?.()}
                          className="w-full flex items-center gap-half px-base py-half text-sm text-low hover:text-normal transition-colors"
                        >
                          <CaretRightIcon
                            className={`size-icon-xs transition-transform ${showPlanningMessages ? 'rotate-90' : ''}`}
                          />
                          <span>{t('common:workspaces.planningConversation')}</span>
                          <span className="text-xs text-low ml-auto">
                            {planningMessages.length} {t('common:workspaces.messages')}
                          </span>
                        </button>
                        {showPlanningMessages && (
                          <div className="px-base pb-base space-y-half">
                            {planningMessages.map((msg) => (
                              <div
                                key={msg.id}
                                className={`rounded p-half text-sm ${
                                  msg.role === 'user'
                                    ? 'bg-brand/10 text-high'
                                    : 'bg-secondary text-normal'
                                }`}
                              >
                                <div className="text-xs text-low mb-px font-medium">
                                  {msg.role === 'user' ? t('common:workspaces.planningUser') : t('common:workspaces.planningPlanner')}
                                </div>
                                <div className="whitespace-pre-wrap break-words">
                                  {msg.content}
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    )}
                    <RetryUiProvider>
                      <ConversationList attempt={workspaceWithSession} />
                    </RetryUiProvider>
                  </div>
                </div>
              );
            })()}
            {/* Chat box - always rendered to prevent flash during workspace switch */}
            <div className="flex justify-center @container pl-px">
              <SessionChatBoxContainer
                session={session}
                sessions={sessions}
                onSelectSession={onSelectSession}
                filesChanged={diffStats?.filesChanged}
                linesAdded={diffStats?.linesAdded}
                linesRemoved={diffStats?.linesRemoved}
                projectId={projectId}
                isNewSessionMode={isNewSessionMode}
                onStartNewSession={onStartNewSession}
                workspaceId={workspaceWithSession?.id}
              />
            </div>
          </MessageEditProvider>
        </EntriesProvider>
      </ApprovalFeedbackProvider>
      {/* Context Bar - floating toolbar */}
      {workspaceWithSession && (
        <ContextBarContainer containerRef={containerRef} />
      )}
    </main>
  );
}
