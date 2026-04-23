import {
  PaperPlaneTiltIcon,
  RobotIcon,
  UserIcon,
  WrenchIcon,
  CaretDownIcon,
  PlusIcon,
  ChatCircleIcon,
  ArrowSquareOutIcon,
  GearIcon,
} from '@phosphor-icons/react';
import type { ConciergeMessage, ConciergeSession } from '@/lib/conciergeApi';
import type { WorkflowDetailDto } from 'shared/types';
import { useTranslation } from 'react-i18next';

function workflowStatusClass(status: string): string {
  if (status === 'running' || status === 'completed') return 'bg-success/20 text-success';
  if (status === 'failed') return 'bg-error/20 text-error';
  return 'bg-secondary text-low';
}

function taskDotClass(status: string): string {
  if (status === 'completed') return 'bg-success';
  if (status === 'running') return 'bg-success animate-pulse';
  if (status === 'failed') return 'bg-error';
  return 'bg-secondary';
}

function termDotClass(status: string): string {
  if (status === 'working') return 'bg-success animate-pulse';
  if (status === 'completed') return 'bg-success';
  if (status === 'failed') return 'bg-error';
  if (status === 'waiting') return 'bg-brand';
  return 'bg-secondary';
}

interface SyncToggles {
  readonly syncTools: boolean;
  readonly syncTerminal: boolean;
  readonly syncProgress: boolean;
  readonly notifyOnCompletion: boolean;
}

interface ConciergeChatViewProps {
  readonly messages: readonly ConciergeMessage[];
  readonly isLoading: boolean;
  readonly sessionName: string;
  readonly sessions: readonly ConciergeSession[];
  readonly activeSessionId: string | null;
  readonly onSelectSession: (id: string) => void;
  readonly onCreateSession: () => void;
  readonly inputValue: string;
  readonly onInputChange: (value: string) => void;
  readonly onSubmit: () => void;
  readonly showSessions: boolean;
  readonly onToggleSessions: () => void;
  readonly bottomRef: React.RefObject<HTMLDivElement>;
  readonly activeWorkflowId: string | null;
  readonly workflow: WorkflowDetailDto | null;
  readonly feishuSync?: boolean;
  readonly onToggleFeishuSync?: () => void;
  readonly onSyncHistory?: () => void;
  readonly syncToggles?: SyncToggles;
  readonly onUpdateSyncToggle?: (key: keyof SyncToggles, value: boolean) => void;
}

function SourceBadge({ provider }: { readonly provider: string | null }) {
  const { t } = useTranslation('common');
  if (!provider) return null;
  const label = provider === 'feishu' ? t('concierge.sourceFeishu') : t('concierge.sourceWeb');
  return (
    <span className="inline-flex items-center rounded bg-secondary px-1 py-px text-xs text-low">
      {label}
    </span>
  );
}

function ToolMessage({ message }: { readonly message: ConciergeMessage }) {
  const { t } = useTranslation('common');
  const isCall = message.role === 'tool_call';
  const label = isCall
    ? t('concierge.toolCall', { name: message.toolName ?? 'unknown' })
    : t('concierge.toolResult', { name: message.toolName ?? 'unknown' });

  return (
    <details className="rounded border bg-secondary px-base py-half text-sm text-low">
      <summary className="flex cursor-pointer items-center gap-1 select-none">
        <WrenchIcon className="size-icon-xs shrink-0" />
        <span>{label}</span>
        <CaretDownIcon className="size-icon-xs ml-auto" />
      </summary>
      <pre className="mt-half overflow-x-auto whitespace-pre-wrap font-ibm-plex-mono text-xs text-normal">
        {message.content}
      </pre>
    </details>
  );
}

function MessageBubble({ message }: { readonly message: ConciergeMessage }) {
  if (message.role === 'tool_call' || message.role === 'tool_result') {
    return <ToolMessage message={message} />;
  }

  const isUser = message.role === 'user';
  const isSystem = message.role === 'system';

  if (isSystem) {
    return (
      <div className="mx-auto max-w-md rounded bg-secondary px-base py-half text-center text-xs text-low">
        {message.content}
      </div>
    );
  }

  return (
    <div className={`flex gap-half ${isUser ? 'flex-row-reverse' : 'flex-row'}`}>
      <div
        className={`flex size-6 shrink-0 items-center justify-center rounded-full ${
          isUser ? 'bg-brand/20 text-brand' : 'bg-secondary text-normal'
        }`}
      >
        {isUser ? (
          <UserIcon className="size-icon-xs" weight="bold" />
        ) : (
          <RobotIcon className="size-icon-xs" weight="bold" />
        )}
      </div>
      <div
        className={`max-w-[75%] rounded px-base py-half text-base ${
          isUser ? 'bg-brand/10 text-high' : 'bg-secondary text-normal'
        }`}
      >
        <p className="whitespace-pre-wrap break-words">{message.content}</p>
        <div className="mt-px flex items-center gap-1">
          <SourceBadge provider={message.sourceProvider} />
          <span className="text-xs text-low">
            {new Date(message.createdAt).toLocaleTimeString([], {
              hour: '2-digit',
              minute: '2-digit',
            })}
          </span>
        </div>
      </div>
    </div>
  );
}

function SyncToggleCheckbox({
  label,
  checked,
  onChange,
}: {
  readonly label: string;
  readonly checked: boolean;
  readonly onChange: (v: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-1 cursor-pointer text-xs text-normal select-none">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="accent-brand size-3"
      />
      {label}
    </label>
  );
}

function SyncTogglesPanel({
  toggles,
  onUpdate,
}: {
  readonly toggles: SyncToggles;
  readonly onUpdate: (key: keyof SyncToggles, value: boolean) => void;
}) {
  const { t } = useTranslation('common');
  return (
    <div className="flex flex-wrap items-center gap-base px-base py-half border-b bg-secondary/30">
      <span className="flex items-center gap-1 text-xs text-low">
        <GearIcon className="size-icon-xs" />
        {t('concierge.syncLabel')}
      </span>
      <SyncToggleCheckbox
        label={t('concierge.syncTools')}
        checked={toggles.syncTools}
        onChange={(v) => onUpdate('syncTools', v)}
      />
      <SyncToggleCheckbox
        label={t('concierge.syncTerminal')}
        checked={toggles.syncTerminal}
        onChange={(v) => onUpdate('syncTerminal', v)}
      />
      <SyncToggleCheckbox
        label={t('concierge.syncProgress')}
        checked={toggles.syncProgress}
        onChange={(v) => onUpdate('syncProgress', v)}
      />
      <SyncToggleCheckbox
        label={t('concierge.syncCompletion')}
        checked={toggles.notifyOnCompletion}
        onChange={(v) => onUpdate('notifyOnCompletion', v)}
      />
    </div>
  );
}

function WorkflowProgressPanel({ workflow }: { readonly workflow: WorkflowDetailDto }) {
  const { t } = useTranslation('common');
  const tasks = workflow.tasks ?? [];
  const completedTasks = tasks.filter(t => t.status === 'completed').length;
  const allTerminals = tasks.flatMap(t => t.terminals ?? []);
  const workingTerminals = allTerminals.filter(t => t.status === 'working');

  return (
    <div className="mx-base rounded border bg-secondary/50 px-base py-half">
      <div className="flex items-center gap-half text-sm">
        <span className="font-medium text-normal">{workflow.name}</span>
        <span className={`rounded-full px-1.5 py-px text-xs ${workflowStatusClass(workflow.status)}`}>
          {workflow.status}
        </span>
        <a
          href={`/pipeline/${workflow.id}`}
          className="ml-auto flex items-center gap-1 text-xs text-brand hover:text-brand/80"
        >
          <ArrowSquareOutIcon className="size-icon-xs" />
          {t('concierge.pipeline')}
        </a>
      </div>
      {tasks.length > 0 && (
        <div className="mt-half flex flex-col gap-px">
          <span className="text-xs text-low">
            {t('concierge.tasksProgress', { completed: completedTasks, total: tasks.length })}
          </span>
          {tasks.map(task => (
            <div key={task.id} className="flex items-center gap-half text-xs">
              <span className={`inline-block size-1.5 rounded-full ${taskDotClass(task.status)}`} />
              <span className="truncate text-normal">{task.name}</span>
              {(task.terminals ?? []).length > 0 && (
                <div className="ml-auto flex gap-px">
                  {(task.terminals ?? []).map(term => (
                    <span
                      key={term.id}
                      title={`${term.role ?? 'terminal'}: ${term.status}`}
                      className={`inline-block size-1.5 rounded-full ${termDotClass(term.status)}`}
                    />
                  ))}
                </div>
              )}
            </div>
          ))}
          {workingTerminals.length > 0 && (
            <span className="text-xs text-low">
              {t('concierge.terminalsWorking', { count: workingTerminals.length })}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

export function ConciergeChatView({
  messages,
  isLoading,
  sessionName,
  sessions,
  activeSessionId,
  onSelectSession,
  onCreateSession,
  inputValue,
  onInputChange,
  onSubmit,
  showSessions,
  onToggleSessions,
  bottomRef,
  activeWorkflowId,
  workflow,
  feishuSync = false,
  onToggleFeishuSync,
  onSyncHistory,
  syncToggles,
  onUpdateSyncToggle,
}: ConciergeChatViewProps) {
  const { t } = useTranslation('common');
  return (
    <div className="flex h-full flex-col bg-primary font-ibm-plex-sans">
      {/* Header */}
      <div className="flex items-center gap-base border-b px-base py-half">
        <ChatCircleIcon className="size-icon-sm text-brand" weight="fill" />
        <h2 className="text-lg font-medium text-high">{sessionName}</h2>
        {activeWorkflowId && (
          <a
            href={`/pipeline/${activeWorkflowId}`}
            className="flex items-center gap-1 rounded-full bg-success/20 px-base py-px text-xs text-success hover:bg-success/30 transition-colors"
          >
            <span className="inline-block size-1.5 rounded-full bg-success animate-pulse" />
            <span>{t('concierge.viewWorkflowProgress')}</span>
          </a>
        )}
        {onToggleFeishuSync && (
          <button
            type="button"
            onClick={onToggleFeishuSync}
            className={`flex items-center gap-1 rounded px-half py-px text-xs transition-colors ${
              feishuSync
                ? 'bg-brand/20 text-brand hover:bg-brand/30'
                : 'bg-secondary text-low hover:text-normal'
            }`}
            title={feishuSync ? t('concierge.feishuSyncEnabled') : t('concierge.feishuSyncDisabled')}
          >
            <span className={`inline-block size-1.5 rounded-full ${feishuSync ? 'bg-brand' : 'bg-secondary'}`} />{' '}
            {t('concierge.feishuSync')}
          </button>
        )}
        {onSyncHistory && (
          <button
            type="button"
            onClick={onSyncHistory}
            className="flex items-center gap-1 rounded px-half py-px text-xs bg-secondary text-low hover:text-normal hover:bg-tertiary transition-colors"
            title={t('concierge.syncHistoryTooltip')}
          >
            <ArrowSquareOutIcon className="size-icon-xs" />
            {t('concierge.syncHistory')}
          </button>
        )}
        <div className="relative ml-auto">
          <button
            type="button"
            onClick={onToggleSessions}
            className="flex items-center gap-1 rounded bg-secondary px-half py-px text-sm text-normal hover:text-high"
          >
            {t('concierge.sessionList')}
            <CaretDownIcon className="size-icon-xs" />
          </button>
          {showSessions && (
            <div className="absolute right-0 top-full z-10 mt-1 w-48 rounded border bg-panel shadow-md">
              {sessions.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  onClick={() => onSelectSession(s.id)}
                  className={`block w-full truncate px-base py-half text-left text-sm hover:bg-secondary ${
                    s.id === activeSessionId ? 'text-brand' : 'text-normal'
                  }`}
                >
                  {s.name || s.id.slice(0, 8)}
                </button>
              ))}
              <button
                type="button"
                onClick={onCreateSession}
                className="flex w-full items-center gap-1 border-t px-base py-half text-sm text-low hover:text-normal"
              >
                <PlusIcon className="size-icon-xs" />
                {t('concierge.newSession')}
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Sync Toggles (shown when feishu sync is on) */}
      {feishuSync && syncToggles && onUpdateSyncToggle && (
        <SyncTogglesPanel toggles={syncToggles} onUpdate={onUpdateSyncToggle} />
      )}

      {/* Messages (workflow progress scrolls with conversation) */}
      <div className="flex-1 space-y-base overflow-y-auto p-base">
        {messages.map((msg) => (
          <MessageBubble key={msg.id} message={msg} />
        ))}
        {workflow && <WorkflowProgressPanel workflow={workflow} />}
        {isLoading && (
          <div className="flex items-center gap-half text-sm text-low">
            <RobotIcon className="size-icon-xs animate-pulse" />
            {t('concierge.thinking')}
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          onSubmit();
        }}
        className="flex items-center gap-half border-t px-base py-half"
      >
        <input
          type="text"
          value={inputValue}
          onChange={(e) => onInputChange(e.target.value)}
          placeholder={t('concierge.sendPlaceholder')}
          className="flex-1 rounded bg-secondary px-base py-half text-base text-normal placeholder:text-low focus:outline-none focus:ring-1 focus:ring-brand"
        />
        <button
          type="submit"
          disabled={!inputValue.trim() || isLoading}
          className="flex items-center justify-center rounded bg-brand/90 p-half text-white hover:bg-brand disabled:opacity-40"
        >
          <PaperPlaneTiltIcon className="size-icon-sm" weight="bold" />
        </button>
      </form>
    </div>
  );
}
