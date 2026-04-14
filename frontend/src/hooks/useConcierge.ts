import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  conciergeApi,
  type ConciergeSession,
  type ConciergeMessage,
  type CreateSessionRequest,
  type SendMessageRequest,
  type UpdateSettingsRequest,
} from '@/lib/conciergeApi';

export const conciergeKeys = {
  all: ['concierge'] as const,
  sessions: () => ['concierge', 'sessions'] as const,
  session: (sessionId: string) => ['concierge', 'sessions', sessionId] as const,
  messages: (sessionId: string) =>
    ['concierge', 'sessions', sessionId, 'messages'] as const,
  feishuChannel: () => ['concierge', 'feishu-channel'] as const,
};

/** List all concierge sessions */
export function useConciergeSessions() {
  return useQuery({
    queryKey: conciergeKeys.sessions(),
    queryFn: () => conciergeApi.listSessions(),
    refetchInterval: 10_000, // Poll every 10s to pick up Feishu-created sessions
  });
}

/** Fetch a single concierge session by ID */
export function useConciergeSession(sessionId: string | null) {
  return useQuery({
    queryKey: conciergeKeys.session(sessionId ?? ''),
    queryFn: () => conciergeApi.getSession(sessionId!),
    enabled: !!sessionId,
  });
}

/** Fetch messages for a concierge session */
export function useConciergeMessages(
  sessionId: string | null,
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: conciergeKeys.messages(sessionId ?? ''),
    queryFn: () => conciergeApi.listMessages(sessionId!),
    enabled: !!sessionId,
    refetchInterval: options?.refetchInterval,
  });
}

/** Send a message in a concierge session */
export function useSendConciergeMessage() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      sessionId,
      data,
    }: {
      sessionId: string;
      data: SendMessageRequest;
    }): Promise<ConciergeMessage[]> => {
      return conciergeApi.sendMessage(sessionId, data);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.messages(variables.sessionId),
      });
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.session(variables.sessionId),
      });
    },
  });
}

/** Create a new concierge session */
export function useCreateConciergeSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (
      data: CreateSessionRequest
    ): Promise<ConciergeSession> => {
      return conciergeApi.createSession(data);
    },
    onSuccess: (session) => {
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.sessions(),
      });
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.messages(session.id),
      });
    },
  });
}

/** Update concierge session settings */
export function useUpdateConciergeSettings() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      sessionId,
      data,
    }: {
      sessionId: string;
      data: UpdateSettingsRequest;
    }): Promise<ConciergeSession> => {
      return conciergeApi.updateSettings(sessionId, data);
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.session(result.id),
      });
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.sessions(),
      });
    },
  });
}

/** Fetch the current Feishu channel binding */
export function useFeishuChannel() {
  return useQuery({
    queryKey: conciergeKeys.feishuChannel(),
    queryFn: () => conciergeApi.getFeishuChannel(),
    refetchInterval: 10_000,
  });
}

/** Switch the Feishu channel to a different session */
export function useSwitchFeishuChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (sessionId: string) =>
      conciergeApi.switchFeishuChannel(sessionId),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.feishuChannel(),
      });
      queryClient.invalidateQueries({
        queryKey: conciergeKeys.sessions(),
      });
    },
  });
}
