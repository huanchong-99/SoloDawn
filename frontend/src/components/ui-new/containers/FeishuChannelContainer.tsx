import { useCallback, useState, useEffect } from 'react';

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
        } catch {
          globalThis.alert(
            '绑定失败：未找到飞书聊天。\n\n请先在飞书中给 Bot 发送一条消息。'
          );
        } finally {
          setSwitching(false);
          setSelectedValue('');
        }
      } else {
        switchChannel.mutate(value, {
          onSettled: () => setSelectedValue(''),
        });
      }
    },
    [switchChannel, channel?.chatId]
  );

  const sessionList = (sessions ?? []).map((s) => ({
    id: s.id,
    name: s.name || s.id.slice(0, 8),
  }));

  const draftList = (drafts ?? []).map((d) => ({
    id: `draft:${d.id}`,
    name: `📝 ${d.name?.slice(0, 30) || d.id.slice(0, 8)}`,
  }));

  const allItems = [...sessionList, ...draftList];

  return (
    <FeishuChannelPanel
      connected={feishuConnected}
      activeSessionId={channel?.activeSessionId ?? null}
      activeSessionName={channel?.activeSessionName ?? null}
      sessions={allItems}
      selectedValue={selectedValue}
      onSelectChange={handleSelectChange}
      isPending={switchChannel.isPending || switching}
    />
  );
}
