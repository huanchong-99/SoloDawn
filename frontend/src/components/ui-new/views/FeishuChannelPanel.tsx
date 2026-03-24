import { ChatCircleIcon, LinkSimpleIcon } from '@phosphor-icons/react';

interface FeishuChannelPanelProps {
  readonly connected: boolean;
  readonly activeSessionId: string | null;
  readonly activeSessionName: string | null;
  readonly sessions: readonly { id: string; name: string }[];
  readonly currentSessionId: string | null;
  readonly onSwitchChannel: (sessionId: string) => void;
  readonly isPending: boolean;
}

export function FeishuChannelPanel({
  connected,
  activeSessionId,
  activeSessionName,
  sessions: _sessions,
  currentSessionId,
  onSwitchChannel,
  isPending,
}: FeishuChannelPanelProps) {
  if (!connected) {
    return (
      <div className="border-b px-base py-half">
        <div className="flex items-center gap-half text-xs text-low">
          <ChatCircleIcon className="size-icon-xs" />
          <span>飞书未连接</span>
        </div>
      </div>
    );
  }

  const isCurrentBound = activeSessionId === currentSessionId;

  return (
    <div className="border-b px-base py-half">
      <div className="mb-half flex items-center gap-half text-xs text-low">
        <ChatCircleIcon className="size-icon-xs text-brand" />
        <span className="font-medium text-normal">飞书通道</span>
      </div>

      {/* Current active session */}
      <div className="mb-half flex items-center gap-half text-xs">
        <LinkSimpleIcon className="size-icon-xs shrink-0" />
        <span className="text-low">当前绑定：</span>
        <span className="truncate text-normal">
          {activeSessionName ?? '无'}
        </span>
      </div>

      {/* Bind current session button */}
      {currentSessionId && !isCurrentBound && (
        <button
          type="button"
          onClick={() => onSwitchChannel(currentSessionId)}
          disabled={isPending}
          className="w-full rounded bg-brand/10 px-half py-half text-xs text-brand transition-colors hover:bg-brand/20 disabled:opacity-50"
        >
          {isPending ? '切换中...' : '绑定当前会话到飞书'}
        </button>
      )}
      {isCurrentBound && (
        <div className="rounded bg-success/10 px-half py-half text-center text-xs text-success">
          当前会话已绑定飞书
        </div>
      )}
    </div>
  );
}
