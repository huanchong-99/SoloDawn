import {
  PaperPlaneTiltIcon,
  CheckCircleIcon,
  SpinnerIcon,
  RocketLaunchIcon,
} from '@phosphor-icons/react';
import { useTranslation } from 'react-i18next';
import { ChatBoxBase, VisualVariant, type EditorProps } from './ChatBoxBase';
import { PrimaryButton } from './PrimaryButton';

/** Draft status passed via props (no direct API import in presentational components) */
interface DraftInfo {
  status: 'gathering' | 'spec_ready' | 'confirmed' | 'materialized' | 'cancelled';
}

interface MessageInfo {
  id: string;
  role: 'user' | 'assistant';
  content: string;
}

interface PlanningChatProps {
  /** Current draft (null = initial state, show first message input) */
  draft: DraftInfo | null;
  /** Conversation messages */
  messages: MessageInfo[];
  /** Editor props for the input */
  editor: EditorProps;
  /** Whether the LLM is currently responding */
  isThinking: boolean;
  /** Whether the draft is being confirmed */
  isConfirming: boolean;
  /** Whether the draft is being materialized */
  isMaterializing: boolean;
  /** Project ID for file typeahead */
  projectId?: string;
  /** Callbacks */
  onSend: () => void;
  onConfirm: () => void;
  onMaterialize: () => void;
}

/**
 * Presentational component for the planning chat interface.
 * Shows multi-turn conversation between user and the Workspace Planner agent.
 */
export function PlanningChat({
  draft,
  messages,
  editor,
  isThinking,
  isConfirming,
  isMaterializing,
  projectId,
  onSend,
  onConfirm,
  onMaterialize,
}: Readonly<PlanningChatProps>) {
  const { t } = useTranslation('tasks');

  const canSend = editor.value.trim().length > 0 && !isThinking;
  const isSpecReady = draft?.status === 'spec_ready';
  const isConfirmed = draft?.status === 'confirmed';
  const isMaterialized = draft?.status === 'materialized';

  const handleCmdEnter = () => {
    if (canSend) onSend();
  };

  const getPlaceholder = (): string => {
    if (!draft) return t('conversation.planning.initialPlaceholder');
    if (isSpecReady) return t('conversation.planning.refineOrConfirm');
    return t('conversation.planning.continuePlaceholder');
  };

  // Render the message list when we have a draft
  const renderMessages = () => (
    <div className="flex-1 overflow-y-auto px-double py-base space-y-base">
      {messages.map((msg) => (
        <div
          key={msg.id}
          className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
        >
          <div
            className={`max-w-[80%] rounded-lg px-base py-half text-sm whitespace-pre-wrap ${
              msg.role === 'user'
                ? 'bg-brand/10 text-high'
                : 'bg-secondary text-normal'
            }`}
          >
            {msg.content}
          </div>
        </div>
      ))}
      {isThinking && (
        <div className="flex justify-start">
          <div className="bg-secondary rounded-lg px-base py-half text-sm text-low flex items-center gap-1">
            <SpinnerIcon className="size-icon-sm animate-spin" />
            {t('conversation.planning.thinking')}
          </div>
        </div>
      )}
      <div />
    </div>
  );

  // Action buttons based on draft status
  const renderActions = () => {
    if (isMaterialized) {
      return (
        <PrimaryButton
          disabled
          actionIcon={CheckCircleIcon}
          value={t('conversation.planning.materialized')}
        />
      );
    }
    if (isConfirmed) {
      return (
        <PrimaryButton
          onClick={onMaterialize}
          disabled={isMaterializing}
          actionIcon={isMaterializing ? 'spinner' : RocketLaunchIcon}
          value={t('conversation.planning.materializeButton')}
        />
      );
    }
    if (isSpecReady) {
      return (
        <>
          <PrimaryButton
            onClick={onConfirm}
            disabled={isConfirming}
            actionIcon={isConfirming ? 'spinner' : CheckCircleIcon}
            value={t('conversation.planning.confirmButton')}
          />
          {canSend && (
            <PrimaryButton
              variant="secondary"
              onClick={onSend}
              value={t('conversation.planning.refineButton')}
            />
          )}
        </>
      );
    }
    // gathering state or initial
    return (
      <PrimaryButton
        onClick={onSend}
        disabled={!canSend}
        actionIcon={isThinking ? 'spinner' : undefined}
        value={
          !draft
            ? t('conversation.planning.startPlanning')
            : t('conversation.actions.send')
        }
      />
    );
  };

  // Before any draft exists, show simple input
  if (!draft) {
    return (
      <ChatBoxBase
        editor={editor}
        placeholder={getPlaceholder()}
        onCmdEnter={handleCmdEnter}
        projectId={projectId}
        autoFocus
        visualVariant={VisualVariant.NORMAL}
        headerLeft={
          <span className="flex items-center gap-1 text-sm text-normal font-medium">
            <PaperPlaneTiltIcon className="size-icon-sm" />
            {t('conversation.planning.title')}
          </span>
        }
        footerRight={renderActions()}
      />
    );
  }

  // Draft exists — show conversation + input
  return (
    <div className="flex flex-col h-full">
      {/* Status badge */}
      <div className="px-double py-half border-b flex items-center gap-half">
        <span className="text-xs text-low">{t('conversation.planning.title')}</span>
        <span className="text-xs px-1 py-px rounded bg-brand/10 text-brand">
          {t(`planning.status.${draft.status}`)}
        </span>
      </div>

      {/* Messages */}
      {renderMessages()}

      {/* Input area */}
      {!isMaterialized && (
        <div className="border-t">
          <ChatBoxBase
            editor={editor}
            placeholder={getPlaceholder()}
            onCmdEnter={handleCmdEnter}
            projectId={projectId}
            autoFocus
            visualVariant={
              isSpecReady ? VisualVariant.PLAN : VisualVariant.NORMAL
            }
            footerRight={renderActions()}
          />
        </div>
      )}
    </div>
  );
}
