import { ChatCircleIcon, LinkSimpleIcon } from '@phosphor-icons/react';

interface FeishuChannelPanelProps {
  readonly connected: boolean;
  readonly activeSessionId: string | null;
  readonly activeSessionName: string | null;
  readonly sessions: readonly { id: string; name: string }[];
  readonly selectedValue: string;
  readonly onSelectChange: (value: string) => void;
  readonly isPending: boolean;
}

export function FeishuChannelPanel({
  connected,
  activeSessionId,
  activeSessionName,
  sessions,
  selectedValue,
  onSelectChange,
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

  return (
    <div className="border-b px-base py-half">
      <div className="mb-half flex items-center gap-half text-xs text-low">
        <ChatCircleIcon className="size-icon-xs text-brand" />
        <span className="font-medium text-normal">飞书通道</span>
      </div>

      {/* Current binding */}
      <div className="mb-half flex items-center gap-half text-xs">
        <LinkSimpleIcon className="size-icon-xs shrink-0" />
        <span className="text-low">当前绑定：</span>
        <span className="truncate text-normal">
          {activeSessionName ?? '无'}
        </span>
      </div>

      {/* Session selector */}
      {sessions.length > 0 && (
        <select
          className="w-full rounded border bg-secondary px-half py-half text-xs text-normal"
          value={selectedValue}
          disabled={isPending}
          onChange={(e) => onSelectChange(e.target.value)}
        >
          <option value="">切换绑定会话...</option>
          {sessions.map((s) => (
            <option key={s.id} value={s.id}>
              {s.id === activeSessionId ? `✓ ${s.name}` : s.name}
            </option>
          ))}
        </select>
      )}
    </div>
  );
}
