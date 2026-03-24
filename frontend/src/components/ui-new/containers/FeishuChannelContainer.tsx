import { useCallback, useState, useEffect } from 'react';

import { FeishuChannelPanel } from '../views/FeishuChannelPanel';

import {
  useFeishuChannel,
  useSwitchFeishuChannel,
  useConciergeSessions,
} from '@/hooks/useConcierge';
import { feishuApi } from '@/lib/api';

interface FeishuChannelContainerProps {
  readonly currentSessionId: string | null;
}

export function FeishuChannelContainer({
  currentSessionId,
}: FeishuChannelContainerProps) {
  const [feishuConnected, setFeishuConnected] = useState(false);

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
  const switchChannel = useSwitchFeishuChannel();

  const handleSwitch = useCallback(
    (sessionId: string) => {
      switchChannel.mutate(sessionId);
    },
    [switchChannel]
  );

  const sessionList = (sessions ?? []).map((s) => ({
    id: s.id,
    name: s.name || s.id.slice(0, 8),
  }));

  return (
    <FeishuChannelPanel
      connected={feishuConnected}
      activeSessionId={channel?.activeSessionId ?? null}
      activeSessionName={channel?.activeSessionName ?? null}
      sessions={sessionList}
      currentSessionId={currentSessionId}
      onSwitchChannel={handleSwitch}
      isPending={switchChannel.isPending}
    />
  );
}
