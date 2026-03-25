import { useCallback, useState, useEffect, useMemo } from 'react';

import { FeishuChannelPanel } from '../views/FeishuChannelPanel';

import {
  useFeishuChannel,
  useSwitchFeishuChannel,
  useConciergeSessions,
} from '@/hooks/useConcierge';
import { usePlanningDrafts } from '@/hooks/usePlanningDraft';
import { feishuApi, planningDraftsApi } from '@/lib/api';

export function FeishuChannelContainer() {
  const [feishuConnected, setFeishuConnected] = useState(false);
  const [selectedValue, setSelectedValue] = useState('');
  const [switching, setSwitching] = useState(false);
  // Local override for display after switching to a draft
  const [localBound, setLocalBound] = useState<{
    id: string;
    name: string;
  } | null>(null);

  useEffect(() => {
    feishuApi
      .getStatus()
      .then((status) => {
        setFeishuConnected(status.connectionStatus === 'connected');
      })
      .catch(() => setFeishuConnected(false));
  }, []);

  const { data: channel } = useFeishuChannel();
  const { data: sessions } = useConciergeSessions();
  const { data: drafts } = usePlanningDrafts();
  const switchChannel = useSwitchFeishuChannel();

  const allItems = useMemo(() => {
    const sessionList = (sessions ?? []).map((s) => ({
      id: s.id,
      name: s.name || s.id.slice(0, 8),
    }));
    const draftList = (drafts ?? []).map((d) => ({
      id: `draft:${d.id}`,
      name: `📝 ${d.name?.slice(0, 30) || d.id.slice(0, 8)}`,
    }));
    return [...sessionList, ...draftList];
  }, [sessions, drafts]);

  const handleSelectChange = useCallback(
    async (value: string) => {
      if (!value) return;
      setSelectedValue(value);

      const knownChatId = channel?.chatId ?? undefined;
      if (value.startsWith('draft:')) {
        const draftId = value.slice(6);
        setSwitching(true);
        try {
          await planningDraftsApi.toggleFeishuSync(draftId, {
            enabled: true,
            syncHistory: false,
            chatId: knownChatId,
          });
          // Update local display
          const matched = allItems.find((s) => s.id === value);
          setLocalBound(
            matched ? { id: value, name: matched.name } : null
          );
        } catch {
          globalThis.alert(
            '绑定失败：未找到飞书聊天。\n\n请先在飞书中给 Bot 发送一条消息。'
          );
        } finally {
          setSwitching(false);
          setSelectedValue('');
        }
      } else {
        setLocalBound(null); // Clear override, let server data take over
        switchChannel.mutate(value, {
          onSettled: () => setSelectedValue(''),
        });
      }
    },
    [switchChannel, channel?.chatId, allItems]
  );

  // Resolve displayed active session: local override > server data
  const displayId = localBound?.id ?? channel?.activeSessionId ?? null;
  const displayName = localBound?.name ?? channel?.activeSessionName ?? null;

  return (
    <FeishuChannelPanel
      connected={feishuConnected}
      activeSessionId={displayId}
      activeSessionName={displayName}
      sessions={allItems}
      selectedValue={selectedValue}
      onSelectChange={handleSelectChange}
      isPending={switchChannel.isPending || switching}
    />
  );
}
