import { useState, useEffect, useCallback, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  useConciergeMessages,
  useSendConciergeMessage,
  useCreateConciergeSession,
  useConciergeSessions,
  useUpdateConciergeSettings,
  conciergeKeys,
} from '@/hooks/useConcierge';
import { useConciergeWsStore } from '@/stores/conciergeWsStore';
import { useWorkflow } from '@/hooks/useWorkflows';
import { useWorkflowInvalidation } from '@/hooks/useWorkflowInvalidation';
import { useCreateMode } from '@/contexts/CreateModeContext';
import { ConciergeChatView } from '../views/ConciergeChatView';

interface ConciergeChatContainerProps {
  readonly initialSessionId?: string | null;
}

export function ConciergeChatContainer({
  initialSessionId,
}: ConciergeChatContainerProps = {}) {
  const [activeSessionId, setActiveSessionId] = useState<string | null>(
    initialSessionId ?? null
  );
  const [inputValue, setInputValue] = useState('');
  const [showSessions, setShowSessions] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const queryClient = useQueryClient();
  const wsConnectedRef = useRef(false);

  // Queries
  const { data: sessions } = useConciergeSessions();
  const { data: messages, isLoading: messagesLoading } =
    useConciergeMessages(activeSessionId);

  // Mutations
  const sendMessage = useSendConciergeMessage();
  const createSession = useCreateConciergeSession();

  // Sync with initialSessionId prop changes
  useEffect(() => {
    if (initialSessionId) {
      setActiveSessionId(initialSessionId);
    }
  }, [initialSessionId]);

  // Auto-select the first session if none is active
  useEffect(() => {
    if (!activeSessionId && sessions && sessions.length > 0) {
      setActiveSessionId(sessions[0].id);
    }
  }, [activeSessionId, sessions]);

  // Auto-scroll on new messages
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages?.length]);

  // WebSocket connection management
  const wsConnect = useConciergeWsStore((s) => s.connect);
  const wsDisconnect = useConciergeWsStore((s) => s.disconnect);
  const wsSubscribe = useConciergeWsStore((s) => s.subscribe);

  useEffect(() => {
    if (!activeSessionId) return;

    wsConnect(activeSessionId);
    wsConnectedRef.current = true;

    return () => {
      if (wsConnectedRef.current) {
        wsDisconnect(activeSessionId);
        wsConnectedRef.current = false;
      }
    };
  }, [activeSessionId, wsConnect, wsDisconnect]);

  // Invalidate messages on WebSocket events
  useEffect(() => {
    if (!activeSessionId) return;

    const unsub = wsSubscribe('concierge.message', () => {
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.messages(activeSessionId),
      });
    });
    return unsub;
  }, [activeSessionId, wsSubscribe, queryClient]);

  // Handlers
  const handleSubmit = useCallback(() => {
    if (!activeSessionId || !inputValue.trim()) return;
    sendMessage.mutate({
      sessionId: activeSessionId,
      data: { message: inputValue.trim(), source: 'web' },
    });
    setInputValue('');
  }, [activeSessionId, inputValue, sendMessage]);

  const handleCreateSession = useCallback(() => {
    createSession.mutate(
      {},
      {
        onSuccess: (session) => {
          setActiveSessionId(session.id);
          setShowSessions(false);
        },
      }
    );
  }, [createSession]);

  const handleSelectSession = useCallback((id: string) => {
    setActiveSessionId(id);
    setShowSessions(false);
  }, []);

  const handleToggleSessions = useCallback(() => {
    setShowSessions((v) => !v);
  }, []);

  // Find active session name
  const activeSession = sessions?.find((s) => s.id === activeSessionId);

  // Sync right sidebar project when switching concierge sessions
  const { selectedProjectId, setSelectedProjectId } = useCreateMode();
  useEffect(() => {
    if (activeSession?.activeProjectId && activeSession.activeProjectId !== selectedProjectId) {
      setSelectedProjectId(activeSession.activeProjectId);
    }
  }, [activeSession?.activeProjectId, selectedProjectId, setSelectedProjectId]);

  // Feishu sync toggle
  const updateSettings = useUpdateConciergeSettings();
  const handleToggleFeishuSync = useCallback(() => {
    if (!activeSessionId || !activeSession) return;
    updateSettings.mutate({
      sessionId: activeSessionId,
      data: { feishuSync: !activeSession.feishuSync },
    });
  }, [activeSessionId, activeSession, updateSettings]);
  const activeWorkflowId = activeSession?.activeWorkflowId ?? null;
  const { data: workflow } = useWorkflow(activeWorkflowId ?? '');
  useWorkflowInvalidation(activeWorkflowId ?? undefined);
  const sessionName = activeSession?.name || 'AI Assistant';

  // Sync toggle handler
  const handleUpdateSyncToggle = useCallback(
    (key: 'syncTools' | 'syncTerminal' | 'syncProgress' | 'notifyOnCompletion', value: boolean) => {
      if (!activeSessionId) return;
      updateSettings.mutate({
        sessionId: activeSessionId,
        data: { [key]: value },
      });
    },
    [activeSessionId, updateSettings]
  );

  const syncToggles = activeSession
    ? {
        syncTools: activeSession.syncTools ?? false,
        syncTerminal: activeSession.syncTerminal ?? false,
        syncProgress: activeSession.syncProgress ?? false,
        notifyOnCompletion: activeSession.notifyOnCompletion ?? true,
      }
    : undefined;

  return (
    <ConciergeChatView
      messages={messages ?? []}
      isLoading={messagesLoading || sendMessage.isPending}
      sessionName={sessionName}
      sessions={sessions ?? []}
      activeSessionId={activeSessionId}
      onSelectSession={handleSelectSession}
      onCreateSession={handleCreateSession}
      inputValue={inputValue}
      onInputChange={setInputValue}
      onSubmit={handleSubmit}
      showSessions={showSessions}
      onToggleSessions={handleToggleSessions}
      bottomRef={bottomRef}
      activeWorkflowId={activeSession?.activeWorkflowId ?? null}
      workflow={workflow ?? null}
      feishuSync={activeSession?.feishuSync ?? false}
      onToggleFeishuSync={handleToggleFeishuSync}
      syncToggles={syncToggles}
      onUpdateSyncToggle={handleUpdateSyncToggle}
    />
  );
}
