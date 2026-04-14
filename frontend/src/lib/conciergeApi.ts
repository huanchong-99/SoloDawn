import { makeRequest, handleApiResponse } from '@/lib/api';

// ── Types ──────────────────────────────────────────────────────────────────

export interface ConciergeSession {
  id: string;
  name: string;
  activeProjectId: string | null;
  activeWorkflowId: string | null;
  feishuSync: boolean;
  feishuChatId: string | null;
  progressNotifications: boolean;
  syncTools: boolean;
  syncTerminal: boolean;
  syncProgress: boolean;
  notifyOnCompletion: boolean;
  llmModelId: string | null;
  llmApiType: string | null;
  llmBaseUrl: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ConciergeMessage {
  id: string;
  sessionId: string;
  role: 'user' | 'assistant' | 'tool_call' | 'tool_result' | 'system';
  content: string;
  sourceProvider: string | null;
  sourceUser: string | null;
  toolName: string | null;
  toolCallId: string | null;
  createdAt: string;
}

export interface CreateSessionRequest {
  name?: string;
  activeProjectId?: string;
  llmModelId?: string;
  llmApiType?: string;
  llmBaseUrl?: string;
}

export interface SendMessageRequest {
  message: string;
  source?: string;
}

export interface UpdateSettingsRequest {
  feishuSync?: boolean;
  syncHistory?: boolean;
  progressNotifications?: boolean;
  syncTools?: boolean;
  syncTerminal?: boolean;
  syncProgress?: boolean;
  notifyOnCompletion?: boolean;
  llmModelId?: string | null;
  llmApiType?: string | null;
  llmBaseUrl?: string | null;
}

export interface AddChannelRequest {
  provider: string;
  externalId: string;
  userIdentifier?: string;
}

// ── API ────────────────────────────────────────────────────────────────────

const BASE = '/api/concierge';

export const conciergeApi = {
  createSession: async (
    data: CreateSessionRequest
  ): Promise<ConciergeSession> => {
    const response = await makeRequest(`${BASE}/sessions`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
    return handleApiResponse<ConciergeSession>(response);
  },

  deleteSession: async (sessionId: string): Promise<void> => {
    const response = await makeRequest(`${BASE}/sessions/${sessionId}`, {
      method: 'DELETE',
    });
    return handleApiResponse<void>(response);
  },

  listSessions: async (): Promise<ConciergeSession[]> => {
    const response = await makeRequest(`${BASE}/sessions`);
    return handleApiResponse<ConciergeSession[]>(response);
  },

  getSession: async (sessionId: string): Promise<ConciergeSession> => {
    const response = await makeRequest(`${BASE}/sessions/${sessionId}`);
    return handleApiResponse<ConciergeSession>(response);
  },

  sendMessage: async (
    sessionId: string,
    data: SendMessageRequest
  ): Promise<ConciergeMessage[]> => {
    const response = await makeRequest(
      `${BASE}/sessions/${sessionId}/messages`,
      {
        method: 'POST',
        body: JSON.stringify(data),
      }
    );
    const result = await handleApiResponse<{
      assistantMessage: string;
      messages: ConciergeMessage[];
    }>(response);
    return result.messages;
  },

  listMessages: async (sessionId: string): Promise<ConciergeMessage[]> => {
    const response = await makeRequest(
      `${BASE}/sessions/${sessionId}/messages`
    );
    return handleApiResponse<ConciergeMessage[]>(response);
  },

  addChannel: async (
    sessionId: string,
    data: AddChannelRequest
  ): Promise<void> => {
    const response = await makeRequest(
      `${BASE}/sessions/${sessionId}/channels`,
      {
        method: 'POST',
        body: JSON.stringify(data),
      }
    );
    return handleApiResponse<void>(response);
  },

  removeChannel: async (
    sessionId: string,
    channelType: string,
    channelId: string
  ): Promise<void> => {
    const response = await makeRequest(
      `${BASE}/sessions/${sessionId}/channels/${channelType}/${channelId}`,
      { method: 'DELETE' }
    );
    return handleApiResponse<void>(response);
  },

  getFeishuChannel: async (): Promise<{
    activeSessionId: string | null;
    activeSessionName: string | null;
    chatId: string | null;
  }> => {
    const response = await makeRequest(`${BASE}/sessions/feishu-channel`);
    return handleApiResponse<{
      activeSessionId: string | null;
      activeSessionName: string | null;
      chatId: string | null;
    }>(response);
  },

  switchFeishuChannel: async (
    sessionId: string
  ): Promise<{
    activeSessionId: string | null;
    activeSessionName: string | null;
    chatId: string | null;
  }> => {
    const response = await makeRequest(`${BASE}/sessions/feishu-channel`, {
      method: 'POST',
      body: JSON.stringify({ sessionId }),
    });
    return handleApiResponse<{
      activeSessionId: string | null;
      activeSessionName: string | null;
      chatId: string | null;
    }>(response);
  },

  updateSettings: async (
    sessionId: string,
    data: UpdateSettingsRequest
  ): Promise<ConciergeSession> => {
    const response = await makeRequest(
      `${BASE}/sessions/${sessionId}/settings`,
      {
        method: 'PATCH',
        body: JSON.stringify(data),
      }
    );
    return handleApiResponse<ConciergeSession>(response);
  },
};
