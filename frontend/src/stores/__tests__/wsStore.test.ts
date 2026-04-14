import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock WebSocket
class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readyState = MockWebSocket.CONNECTING;
  onopen: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor(public url: string) {
    // Simulate async connection - use queueMicrotask for immediate but async execution
    queueMicrotask(() => {
      if (this.readyState === MockWebSocket.CONNECTING) {
        this.readyState = MockWebSocket.OPEN;
        this.onopen?.(new Event('open'));
      }
    });
  }

  send = vi.fn();
  close = vi.fn(() => {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.(new CloseEvent('close'));
  });

  simulateMessage(data: unknown) {
    this.onmessage?.(
      new MessageEvent('message', { data: JSON.stringify(data) })
    );
  }

  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.(new CloseEvent('close'));
  }

  simulateError() {
    this.onerror?.(new Event('error'));
  }
}

const websocketInstances: MockWebSocket[] = [];

// Store the original WebSocket
const originalWebSocket = globalThis.WebSocket;

describe('wsStore', () => {
  let useWsStore: typeof import('../wsStore').useWsStore;

  afterEach(() => {
    vi.useRealTimers();
  });

  beforeEach(async () => {
    vi.useFakeTimers();
    // Mock WebSocket globally
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket;
    websocketInstances.length = 0;

    const OriginalCtor = MockWebSocket as unknown as new (url: string) => MockWebSocket;

    globalThis.WebSocket = class extends OriginalCtor {
      constructor(url: string) {
        super(url);
        websocketInstances.push(this);
      }
    } as unknown as typeof WebSocket;

    // Reset modules to get fresh store
    vi.resetModules();
    const module = await import('../wsStore');
    useWsStore = module.useWsStore;
  });

  afterEach(() => {
    // Clean up store before restoring timers
    const state = useWsStore.getState();
    if (state._heartbeatInterval) {
      clearInterval(state._heartbeatInterval);
    }
    if (state._reconnectTimeout) {
      clearTimeout(state._reconnectTimeout);
    }

    state._workflowConnections.forEach((connection) => {
      if (connection.heartbeatInterval) {
        clearInterval(connection.heartbeatInterval);
      }
      if (connection.reconnectTimeout) {
        clearTimeout(connection.reconnectTimeout);
      }
    });

    vi.useRealTimers();
    globalThis.WebSocket = originalWebSocket;
    vi.clearAllMocks();
  });

  it('should start with disconnected status', () => {
    const state = useWsStore.getState();
    expect(state.connectionStatus).toBe('disconnected');
    expect(state.lastHeartbeat).toBeNull();
    expect(state.reconnectAttempts).toBe(0);
  });

  it('should connect to WebSocket server', async () => {
    const { connect } = useWsStore.getState();

    connect('ws://localhost:8080');

    expect(useWsStore.getState().connectionStatus).toBe('connecting');

    // Wait for microtask to complete (WebSocket connection)
    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });
  });

  it('should send messages when connected', async () => {
    const { connect, send } = useWsStore.getState();

    connect('ws://localhost:8080');

    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });

    const message = {
      type: 'terminal.input',
      payload: { data: 'test' },
      timestamp: new Date().toISOString(),
      id: 'msg-1',
    };

    const sent = send(message);
    expect(sent).toBe(true);

    const ws = useWsStore.getState()._ws as unknown as MockWebSocket;
    expect(ws.send).toHaveBeenCalledWith(JSON.stringify(message));
  });

  it('should dispatch messages to correct handlers', async () => {
    const { connect, subscribe } = useWsStore.getState();

    connect('ws://localhost:8080');

    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });

    const handler = vi.fn();
    const unsubscribe = subscribe('terminal.output', handler);

    const ws = useWsStore.getState()._ws as unknown as MockWebSocket;
    ws.simulateMessage({
      type: 'terminal.output',
      payload: { data: 'hello' },
      timestamp: new Date().toISOString(),
      id: 'msg-1',
    });

    expect(handler).toHaveBeenCalledWith({ data: 'hello' });

    unsubscribe();
  });

  it('should reconnect on disconnect with exponential backoff', async () => {
    const { connect } = useWsStore.getState();

    connect('ws://localhost:8080');

    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });

    // Simulate disconnect
    const ws = useWsStore.getState()._ws as unknown as MockWebSocket;
    ws.simulateClose();

    expect(useWsStore.getState().connectionStatus).toBe('reconnecting');
    expect(useWsStore.getState().reconnectAttempts).toBe(1);

    // Wait for reconnect attempt (1000ms * 2^0 = 1000ms)
    vi.advanceTimersByTime(1000);

    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });
  });

  it('should send heartbeat every 30 seconds', async () => {
    const { connect } = useWsStore.getState();

    connect('ws://localhost:8080');

    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });

    const ws = useWsStore.getState()._ws as unknown as MockWebSocket;

    // Advance 30 seconds
    vi.advanceTimersByTime(30000);

    expect(ws.send).toHaveBeenCalledWith(
      expect.stringContaining('"type":"system.heartbeat"')
    );
  });

  it('should clean up on disconnect', async () => {
    const { connect, disconnect } = useWsStore.getState();

    connect('ws://localhost:8080');

    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });

    const ws = useWsStore.getState()._ws as unknown as MockWebSocket;

    disconnect();

    expect(ws.close).toHaveBeenCalled();
    expect(useWsStore.getState().connectionStatus).toBe('disconnected');
    expect(useWsStore.getState()._ws).toBeNull();
  });

  it('should not reconnect after manual disconnect', async () => {
    const { connect, disconnect } = useWsStore.getState();

    connect('ws://localhost:8080');

    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });

    disconnect();

    // Wait for potential reconnect
    vi.advanceTimersByTime(5000);

    expect(useWsStore.getState().connectionStatus).toBe('disconnected');
    expect(useWsStore.getState().reconnectAttempts).toBe(0);
  });

  it('should handle multiple subscribers for same event', async () => {
    const { connect, subscribe } = useWsStore.getState();

    connect('ws://localhost:8080');

    await vi.waitFor(() => {
      expect(useWsStore.getState().connectionStatus).toBe('connected');
    });

    const handler1 = vi.fn();
    const handler2 = vi.fn();

    subscribe('workflow.updated', handler1);
    subscribe('workflow.updated', handler2);

    const ws = useWsStore.getState()._ws as unknown as MockWebSocket;
    ws.simulateMessage({
      type: 'workflow.updated',
      payload: { id: 'wf-1' },
      timestamp: new Date().toISOString(),
      id: 'msg-1',
    });

    expect(handler1).toHaveBeenCalledWith({ id: 'wf-1' });
    expect(handler2).toHaveBeenCalledWith({ id: 'wf-1' });
  });

  it('supports parallel workflow subscriptions without cross-dispatch', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-a');
    connectToWorkflow('wf-b');

    await vi.waitFor(() => {
      expect(useWsStore.getState().getWorkflowConnectionStatus('wf-a')).toBe(
        'connected'
      );
      expect(useWsStore.getState().getWorkflowConnectionStatus('wf-b')).toBe(
        'connected'
      );
    });

    const handlerA = vi.fn();
    const handlerB = vi.fn();

    subscribeToWorkflow('wf-a', 'terminal.status_changed', handlerA);
    subscribeToWorkflow('wf-b', 'terminal.status_changed', handlerB);

    const wsA = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-a/')
    );
    const wsB = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-b/')
    );

    expect(wsA).toBeDefined();
    expect(wsB).toBeDefined();

    wsA!.simulateMessage({
      type: 'terminal.status_changed',
      payload: { workflowId: 'wf-a', terminalId: 'ta', status: 'working' },
      timestamp: new Date().toISOString(),
      id: 'msg-a',
    });

    expect(handlerA).toHaveBeenCalledTimes(1);
    expect(handlerB).toHaveBeenCalledTimes(0);

    wsB!.simulateMessage({
      type: 'terminal.status_changed',
      payload: { workflowId: 'wf-b', terminalId: 'tb', status: 'working' },
      timestamp: new Date().toISOString(),
      id: 'msg-b',
    });

    expect(handlerA).toHaveBeenCalledTimes(1);
    expect(handlerB).toHaveBeenCalledTimes(1);
  });

  it('disconnectWorkflow uses reference counting', async () => {
    const { connectToWorkflow, disconnectWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-ref');
    connectToWorkflow('wf-ref');

    await vi.waitFor(() => {
      expect(useWsStore.getState().getWorkflowConnectionStatus('wf-ref')).toBe(
        'connected'
      );
    });

    const stateAfterConnect = useWsStore.getState();
    const connectionAfterConnect =
      stateAfterConnect._workflowConnections.get('wf-ref');
    expect(connectionAfterConnect?.refCount).toBe(2);

    disconnectWorkflow('wf-ref');

    const stateAfterOneDisconnect = useWsStore.getState();
    const connectionAfterOneDisconnect =
      stateAfterOneDisconnect._workflowConnections.get('wf-ref');
    expect(connectionAfterOneDisconnect?.refCount).toBe(1);
    expect(stateAfterOneDisconnect.getWorkflowConnectionStatus('wf-ref')).toBe(
      'connected'
    );

    disconnectWorkflow('wf-ref');

    const stateAfterSecondDisconnect = useWsStore.getState();
    expect(stateAfterSecondDisconnect._workflowConnections.has('wf-ref')).toBe(
      false
    );
    expect(
      stateAfterSecondDisconnect.getWorkflowConnectionStatus('wf-ref')
    ).toBe('disconnected');
  });

  it('routes terminal.prompt_* events through workflow-scoped handlers', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-prompt');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-prompt')
      ).toBe('connected');
    });

    const onDetected = vi.fn();
    const onDecision = vi.fn();

    subscribeToWorkflow('wf-prompt', 'terminal.prompt_detected', onDetected);
    subscribeToWorkflow('wf-prompt', 'terminal.prompt_decision', onDecision);

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-prompt/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflowId: 'wf-prompt',
        terminalId: 'term-1',
        promptKind: 'Confirmation',
        promptText: 'Proceed? [y/N]',
        confidence: 0.95,
        hasDangerousKeywords: false,
        options: ['y', 'n'],
        selectedIndex: null,
      },
      timestamp: new Date().toISOString(),
      id: 'msg-detected',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflowId: 'wf-prompt',
        terminalId: 'term-1',
        decision: 'auto_confirm',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-decision',
    });

    expect(onDetected).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-prompt',
        terminalId: 'term-1',
      })
    );
    expect(onDecision).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-prompt',
        decision: 'auto_confirm',
      })
    );
  });

  it('normalizes terminal.completed payload from snake_case and camelCase formats', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-completed');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-completed')
      ).toBe('connected');
    });

    const onCompleted = vi.fn();
    subscribeToWorkflow('wf-completed', 'terminal.completed', onCompleted);

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-completed/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.completed',
      payload: {
        workflowId: 'wf-completed',
        taskId: 'task-1',
        terminalId: 'term-1',
        status: 'review_passed',
        commitHash: 'abc123',
        commitMessage: 'fix: apply patch',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-completed-camel',
    });

    ws!.simulateMessage({
      type: 'terminal.completed',
      payload: {
        workflow_id: 'wf-completed',
        task_id: 'task-1',
        terminal_id: 'term-1',
        status: 'review_pass',
        commit_hash: 'abc123',
        commit_message: 'fix: apply patch',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-completed-snake',
    });

    expect(onCompleted).toHaveBeenCalledTimes(2);

    const firstPayload = onCompleted.mock.calls[0][0];
    const secondPayload = onCompleted.mock.calls[1][0];

    expect(firstPayload).toEqual(
      expect.objectContaining({
        workflowId: 'wf-completed',
        taskId: 'task-1',
        terminalId: 'term-1',
        status: 'review_pass',
        commitHash: 'abc123',
        commitMessage: 'fix: apply patch',
      })
    );
    expect(secondPayload).toEqual(firstPayload);
  });

  it('normalizes terminal.prompt_detected payload with object options', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-prompt-norm');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-prompt-norm')
      ).toBe('connected');
    });

    const onDetected = vi.fn();
    subscribeToWorkflow(
      'wf-prompt-norm',
      'terminal.prompt_detected',
      onDetected
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-prompt-norm/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflow_id: 'wf-prompt-norm',
        terminal_id: 'term-2',
        promptKind: 'ArrowSelect',
        prompt_text: 'Select one option',
        confidence: 0.88,
        has_dangerous_keywords: false,
        options: [
          { index: 0, label: 'Option A', selected: false },
          { index: 1, label: 'Option B', selected: true },
        ],
        selected_index: null,
      },
      timestamp: new Date().toISOString(),
      id: 'msg-prompt-detected',
    });

    expect(onDetected).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-prompt-norm',
        terminalId: 'term-2',
        promptKind: 'arrow_select',
        promptText: 'Select one option',
        options: ['Option A', 'Option B'],
        selectedIndex: 1,
      })
    );

    expect(onDetected).toHaveBeenCalledWith(
      expect.objectContaining({
        optionDetails: [
          { index: 0, label: 'Option A', selected: false },
          { index: 1, label: 'Option B', selected: true },
        ],
      })
    );
  });

  it('normalizes terminal.prompt_detected payload with dual-format server contract fields', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-prompt-contract');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-prompt-contract')
      ).toBe('connected');
    });

    const onDetected = vi.fn();
    subscribeToWorkflow(
      'wf-prompt-contract',
      'terminal.prompt_detected',
      onDetected
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-prompt-contract/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflowId: 'wf-prompt-contract',
        terminal_id: 'term-contract',
        prompt_kind: 'arrow_select',
        promptText: 'Select contract option',
        confidence: 0.91,
        hasDangerousKeywords: false,
        options: ['Option A', 'Option B'],
        optionDetails: [
          { index: 0, label: 'Option A', selected: false },
          { index: 1, label: 'Option B', selected: true },
        ],
      },
      timestamp: new Date().toISOString(),
      id: 'msg-prompt-contract',
    });

    expect(onDetected).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-prompt-contract',
        terminalId: 'term-contract',
        promptKind: 'arrow_select',
        promptText: 'Select contract option',
        options: ['Option A', 'Option B'],
        selectedIndex: 1,
        optionDetails: [
          { index: 0, label: 'Option A', selected: false },
          { index: 1, label: 'Option B', selected: true },
        ],
      })
    );
  });

  it('normalizes terminal.prompt_detected task/session context from camelCase and snake_case', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-prompt-context');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-prompt-context')
      ).toBe('connected');
    });

    const onDetected = vi.fn();
    subscribeToWorkflow(
      'wf-prompt-context',
      'terminal.prompt_detected',
      onDetected
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-prompt-context/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflowId: 'wf-prompt-context',
        terminalId: 'term-context',
        taskId: 'task-context',
        sessionId: 'session-context',
        promptKind: 'yes_no',
        promptText: 'Proceed? [y/N]',
        confidence: 0.92,
        hasDangerousKeywords: false,
        options: ['y', 'n'],
        selectedIndex: 1,
      },
      timestamp: new Date().toISOString(),
      id: 'msg-prompt-context-camel',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflow_id: 'wf-prompt-context',
        terminal_id: 'term-context',
        task_id: 'task-context',
        session_id: 'session-context',
        prompt_kind: 'yes_no',
        prompt_text: 'Proceed? [y/N]',
        confidence: 0.92,
        has_dangerous_keywords: false,
        options: ['y', 'n'],
        selected_index: 1,
      },
      timestamp: new Date().toISOString(),
      id: 'msg-prompt-context-snake',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflowId: 'wf-prompt-context',
        terminalId: 'term-context',
        promptKind: 'yes_no',
        promptText: 'Proceed without context? [y/N]',
        confidence: 0.81,
        hasDangerousKeywords: false,
        options: ['y', 'n'],
        selectedIndex: 0,
      },
      timestamp: new Date().toISOString(),
      id: 'msg-prompt-context-legacy',
    });

    expect(onDetected).toHaveBeenCalledTimes(3);

    const firstPayload = onDetected.mock.calls[0][0];
    const secondPayload = onDetected.mock.calls[1][0];
    const legacyPayload = onDetected.mock.calls[2][0];

    expect(firstPayload).toEqual(
      expect.objectContaining({
        workflowId: 'wf-prompt-context',
        terminalId: 'term-context',
        taskId: 'task-context',
        sessionId: 'session-context',
        promptKind: 'yes_no',
        promptText: 'Proceed? [y/N]',
        selectedIndex: 1,
      })
    );
    expect(secondPayload).toEqual(firstPayload);
    expect(legacyPayload).toEqual(
      expect.objectContaining({
        workflowId: 'wf-prompt-context',
        terminalId: 'term-context',
        promptKind: 'yes_no',
        promptText: 'Proceed without context? [y/N]',
        selectedIndex: 0,
      })
    );
    expect(legacyPayload).not.toHaveProperty('taskId');
    expect(legacyPayload).not.toHaveProperty('sessionId');
  });

  it('normalizes terminal.prompt_decision payload for structured decision object', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-decision-norm');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-decision-norm')
      ).toBe('connected');
    });

    const onDecision = vi.fn();
    subscribeToWorkflow(
      'wf-decision-norm',
      'terminal.prompt_decision',
      onDecision
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-decision-norm/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflowId: 'wf-decision-norm',
        terminalId: 'term-3',
        decision: {
          action: 'llm_decision',
          response: 'y\n',
          reasoning: 'Safe to proceed',
          target_index: 2,
        },
      },
      timestamp: new Date().toISOString(),
      id: 'msg-prompt-decision',
    });

    expect(onDecision).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-decision-norm',
        terminalId: 'term-3',
        decision: 'llm_decision',
        decisionDetail: expect.objectContaining({
          action: 'llm_decision',
          response: 'y\n',
          reasoning: 'Safe to proceed',
          targetIndex: 2,
        }),
      })
    );
  });

  it('normalizes terminal.prompt_decision string payload with dual decision detail fields', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-decision-contract');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-decision-contract')
      ).toBe('connected');
    });

    const onDecision = vi.fn();
    subscribeToWorkflow(
      'wf-decision-contract',
      'terminal.prompt_decision',
      onDecision
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-decision-contract/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflow_id: 'wf-decision-contract',
        terminalId: 'term-contract',
        decision: 'llm_decision',
        decision_detail: {
          action: 'llm_decision',
          response: 'y\n',
          reasoning: 'Safe to proceed',
          target_index: 2,
        },
        decisionRaw: {
          action: 'llm_decision',
          response: 'y\n',
          target_index: 2,
        },
      },
      timestamp: new Date().toISOString(),
      id: 'msg-decision-contract',
    });

    expect(onDecision).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-decision-contract',
        terminalId: 'term-contract',
        decision: 'llm_decision',
        decisionDetail: expect.objectContaining({
          action: 'llm_decision',
          response: 'y\n',
          reasoning: 'Safe to proceed',
          targetIndex: 2,
        }),
        decisionRaw: expect.objectContaining({
          action: 'llm_decision',
          target_index: 2,
        }),
      })
    );
  });

  it('normalizes terminal.prompt_decision task/session context from camelCase and snake_case', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-decision-context');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-decision-context')
      ).toBe('connected');
    });

    const onDecision = vi.fn();
    subscribeToWorkflow(
      'wf-decision-context',
      'terminal.prompt_decision',
      onDecision
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-decision-context/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflowId: 'wf-decision-context',
        terminalId: 'term-context',
        taskId: 'task-context',
        sessionId: 'session-context',
        decision: 'auto_confirm',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-decision-context-camel',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflow_id: 'wf-decision-context',
        terminal_id: 'term-context',
        task_id: 'task-context',
        session_id: 'session-context',
        decision: 'auto_confirm',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-decision-context-snake',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflowId: 'wf-decision-context',
        terminalId: 'term-context',
        decision: 'auto_confirm',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-decision-context-legacy',
    });

    expect(onDecision).toHaveBeenCalledTimes(3);

    const firstPayload = onDecision.mock.calls[0][0];
    const secondPayload = onDecision.mock.calls[1][0];
    const legacyPayload = onDecision.mock.calls[2][0];

    expect(firstPayload).toEqual(
      expect.objectContaining({
        workflowId: 'wf-decision-context',
        terminalId: 'term-context',
        taskId: 'task-context',
        sessionId: 'session-context',
        decision: 'auto_confirm',
      })
    );
    expect(secondPayload).toEqual(firstPayload);
    expect(legacyPayload).toEqual(
      expect.objectContaining({
        workflowId: 'wf-decision-context',
        terminalId: 'term-context',
        decision: 'auto_confirm',
      })
    );
    expect(legacyPayload).not.toHaveProperty('taskId');
    expect(legacyPayload).not.toHaveProperty('sessionId');
  });

  it('normalizes terminal.prompt_detected unknown prompt kind and mixed contract fields', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-prompt-unknown');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-prompt-unknown')
      ).toBe('connected');
    });

    const onDetected = vi.fn();
    subscribeToWorkflow(
      'wf-prompt-unknown',
      'terminal.prompt_detected',
      onDetected
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-prompt-unknown/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflow_id: 'wf-prompt-unknown',
        terminal_id: 'term-unknown',
        prompt_kind: 'ManualApproval',
        raw_text: 'Run dangerous command?',
        confidence: '0.76',
        has_dangerous_keywords: 'true',
        option_details: [
          { index: '0', label: 'Yes', selected: 'false' },
          { index: '1', label: 'No', selected: 'true' },
        ],
      },
      timestamp: new Date().toISOString(),
      id: 'msg-prompt-unknown',
    });

    expect(onDetected).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-prompt-unknown',
        terminalId: 'term-unknown',
        promptKind: 'unknown',
        promptKindRaw: 'ManualApproval',
        promptText: 'Run dangerous command?',
        confidence: 0.76,
        hasDangerousKeywords: true,
        options: ['Yes', 'No'],
        selectedIndex: 1,
        optionDetails: [
          { index: 0, label: 'Yes', selected: false },
          { index: 1, label: 'No', selected: true },
        ],
      })
    );
  });

  it('normalizes terminal.prompt_decision unknown action and extracts decision detail fields', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-decision-unknown');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-decision-unknown')
      ).toBe('connected');
    });

    const onDecision = vi.fn();
    subscribeToWorkflow(
      'wf-decision-unknown',
      'terminal.prompt_decision',
      onDecision
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-decision-unknown/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflowId: 'wf-decision-unknown',
        terminal_id: 'term-unknown',
        decision: 'manual_override',
        decision_detail: {
          response: 'n\n',
          target_index: '3',
          suggestions: ['Retry', 1, null, 'Cancel'],
        },
      },
      timestamp: new Date().toISOString(),
      id: 'msg-decision-unknown',
    });

    expect(onDecision).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-decision-unknown',
        terminalId: 'term-unknown',
        decision: 'unknown',
        decisionRaw: 'manual_override',
        decisionDetail: expect.objectContaining({
          response: 'n\n',
          targetIndex: 3,
          suggestions: ['Retry', 'Cancel'],
        }),
      })
    );
  });

  it('keeps P0 WS contract assertions end-to-end across all terminal events', async () => {
    const { connectToWorkflow, subscribeToWorkflow } = useWsStore.getState();

    connectToWorkflow('wf-contract-e2e');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-contract-e2e')
      ).toBe('connected');
    });

    const onCompleted = vi.fn();
    const onDetected = vi.fn();
    const onDecision = vi.fn();

    subscribeToWorkflow('wf-contract-e2e', 'terminal.completed', onCompleted);
    subscribeToWorkflow('wf-contract-e2e', 'terminal.prompt_detected', onDetected);
    subscribeToWorkflow('wf-contract-e2e', 'terminal.prompt_decision', onDecision);

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-contract-e2e/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.completed',
      payload: {
        workflowId: 'wf-contract-e2e',
        taskId: 'task-e2e',
        terminalId: 'term-e2e',
        status: 'review_reject',
        commitHash: 'abc123',
        commitMessage: 'feat: contract e2e',
        workflow_id: 'wf-contract-e2e',
        task_id: 'task-e2e',
        terminal_id: 'term-e2e',
        commit_hash: 'abc123',
        commit_message: 'feat: contract e2e',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-contract-completed',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflowId: 'wf-contract-e2e',
        terminalId: 'term-e2e',
        promptKind: 'arrow_select',
        promptText: 'Select contract option',
        confidence: 0.95,
        hasDangerousKeywords: false,
        options: ['Option A', 'Option B'],
        optionDetails: [
          { index: 0, label: 'Option A', selected: false },
          { index: 1, label: 'Option B', selected: true },
        ],
        workflow_id: 'wf-contract-e2e',
        terminal_id: 'term-e2e',
        prompt_kind: 'arrow_select',
        prompt_text: 'Select contract option',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-contract-detected',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflowId: 'wf-contract-e2e',
        terminalId: 'term-e2e',
        decision: 'llm_decision',
        decisionDetail: {
          action: 'llm_decision',
          response: 'y\n',
          reasoning: 'contract e2e',
          target_index: 2,
        },
        decisionRaw: {
          action: 'llm_decision',
          response: 'y\n',
          target_index: 2,
        },
        workflow_id: 'wf-contract-e2e',
        terminal_id: 'term-e2e',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-contract-decision',
    });

    expect(onCompleted).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-contract-e2e',
        taskId: 'task-e2e',
        terminalId: 'term-e2e',
        status: 'review_reject',
      })
    );
    expect(onDetected).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-contract-e2e',
        terminalId: 'term-e2e',
        promptKind: 'arrow_select',
        selectedIndex: 1,
      })
    );
    expect(onDecision).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-contract-e2e',
        terminalId: 'term-e2e',
        decision: 'llm_decision',
        decisionDetail: expect.objectContaining({
          targetIndex: 2,
        }),
      })
    );
  });

  it('covers enter_confirm ask_user flow with user submit and workflow continuation', async () => {
    const { connectToWorkflow, subscribeToWorkflow, sendPromptResponse } =
      useWsStore.getState();

    connectToWorkflow('wf-enter-confirm-flow');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-enter-confirm-flow')
      ).toBe('connected');
    });

    const onDetected = vi.fn();
    const onDecision = vi.fn();
    const onCompleted = vi.fn();

    subscribeToWorkflow(
      'wf-enter-confirm-flow',
      'terminal.prompt_detected',
      onDetected
    );
    subscribeToWorkflow(
      'wf-enter-confirm-flow',
      'terminal.prompt_decision',
      onDecision
    );
    subscribeToWorkflow(
      'wf-enter-confirm-flow',
      'terminal.completed',
      onCompleted
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-enter-confirm-flow/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflow_id: 'wf-enter-confirm-flow',
        terminal_id: 'term-enter',
        task_id: 'task-enter',
        session_id: 'session-enter',
        prompt_kind: 'EnterConfirm',
        prompt_text: 'Press Enter to continue',
        confidence: 0.99,
        has_dangerous_keywords: true,
        options: [],
        selected_index: null,
      },
      timestamp: new Date().toISOString(),
      id: 'msg-enter-detected',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflowId: 'wf-enter-confirm-flow',
        terminalId: 'term-enter',
        taskId: 'task-enter',
        sessionId: 'session-enter',
        decision: {
          action: 'ask_user',
          reason: 'dangerous keyword detected',
          suggestions: ['confirm', 'cancel'],
        },
      },
      timestamp: new Date().toISOString(),
      id: 'msg-enter-decision',
    });

    expect(onDetected).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-enter-confirm-flow',
        terminalId: 'term-enter',
        taskId: 'task-enter',
        sessionId: 'session-enter',
        promptKind: 'enter_confirm',
        promptText: 'Press Enter to continue',
      })
    );
    expect(onDecision).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-enter-confirm-flow',
        terminalId: 'term-enter',
        decision: 'ask_user',
        decisionDetail: expect.objectContaining({
          action: 'ask_user',
          reason: 'dangerous keyword detected',
          suggestions: ['confirm', 'cancel'],
        }),
      })
    );

    ws!.send.mockClear();

    const sent = sendPromptResponse({
      workflowId: 'wf-enter-confirm-flow',
      terminalId: 'term-enter',
      response: '\n',
    });
    expect(sent).toBe(true);

    expect(ws!.send).toHaveBeenCalledTimes(1);

    const [enterSerializedMessage] = ws!.send.mock.calls[0] as [string];
    const enterSubmittedMessage = JSON.parse(enterSerializedMessage) as {
      type: string;
      payload: {
        workflowId: string;
        terminalId: string;
        response: string;
      };
    };

    expect(enterSubmittedMessage).toEqual(
      expect.objectContaining({
        type: 'terminal.prompt_response',
        payload: {
          workflowId: 'wf-enter-confirm-flow',
          terminalId: 'term-enter',
          response: '\n',
        },
      })
    );

    ws!.simulateMessage({
      type: 'terminal.completed',
      payload: {
        workflowId: 'wf-enter-confirm-flow',
        taskId: 'task-enter',
        terminalId: 'term-enter',
        status: 'completed',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-enter-completed',
    });

    expect(onCompleted).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-enter-confirm-flow',
        taskId: 'task-enter',
        terminalId: 'term-enter',
        status: 'completed',
      })
    );
    expect(onCompleted.mock.invocationCallOrder[0]).toBeGreaterThan(
      ws!.send.mock.invocationCallOrder[0]
    );
  });

  it('covers arrow_select ask_user flow with user submit and workflow continuation', async () => {
    const { connectToWorkflow, subscribeToWorkflow, sendPromptResponse } =
      useWsStore.getState();

    connectToWorkflow('wf-arrow-select-flow');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-arrow-select-flow')
      ).toBe('connected');
    });

    const onDetected = vi.fn();
    const onDecision = vi.fn();
    const onCompleted = vi.fn();

    subscribeToWorkflow(
      'wf-arrow-select-flow',
      'terminal.prompt_detected',
      onDetected
    );
    subscribeToWorkflow(
      'wf-arrow-select-flow',
      'terminal.prompt_decision',
      onDecision
    );
    subscribeToWorkflow(
      'wf-arrow-select-flow',
      'terminal.completed',
      onCompleted
    );

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-arrow-select-flow/')
    );
    expect(ws).toBeDefined();

    ws!.simulateMessage({
      type: 'terminal.prompt_detected',
      payload: {
        workflowId: 'wf-arrow-select-flow',
        terminalId: 'term-select',
        taskId: 'task-select',
        sessionId: 'session-select',
        promptKind: 'ArrowSelect',
        promptText: 'Select deployment environment',
        confidence: 0.87,
        hasDangerousKeywords: false,
        optionDetails: [
          { index: 0, label: 'Staging', selected: false },
          { index: 1, label: 'Production', selected: true },
          { index: 2, label: 'Canary', selected: false },
        ],
        selectedIndex: null,
      },
      timestamp: new Date().toISOString(),
      id: 'msg-select-detected',
    });

    ws!.simulateMessage({
      type: 'terminal.prompt_decision',
      payload: {
        workflow_id: 'wf-arrow-select-flow',
        terminal_id: 'term-select',
        task_id: 'task-select',
        session_id: 'session-select',
        decision: 'ask_user',
        decision_detail: {
          action: 'ask_user',
          reason: 'manual environment confirmation required',
          target_index: 2,
          suggestions: ['2', null, '1'],
        },
      },
      timestamp: new Date().toISOString(),
      id: 'msg-select-decision',
    });

    expect(onDetected).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-arrow-select-flow',
        terminalId: 'term-select',
        taskId: 'task-select',
        sessionId: 'session-select',
        promptKind: 'arrow_select',
        promptText: 'Select deployment environment',
        options: ['Staging', 'Production', 'Canary'],
        selectedIndex: 1,
      })
    );
    expect(onDecision).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-arrow-select-flow',
        terminalId: 'term-select',
        taskId: 'task-select',
        sessionId: 'session-select',
        decision: 'ask_user',
        decisionDetail: expect.objectContaining({
          action: 'ask_user',
          reason: 'manual environment confirmation required',
          targetIndex: 2,
          suggestions: ['2', '1'],
        }),
      })
    );

    ws!.send.mockClear();

    const sent = sendPromptResponse({
      workflowId: 'wf-arrow-select-flow',
      terminalId: 'term-select',
      response: '2\n',
    });
    expect(sent).toBe(true);

    expect(ws!.send).toHaveBeenCalledTimes(1);

    const [selectSerializedMessage] = ws!.send.mock.calls[0] as [string];
    const selectSubmittedMessage = JSON.parse(selectSerializedMessage) as {
      type: string;
      payload: {
        workflowId: string;
        terminalId: string;
        response: string;
      };
    };

    expect(selectSubmittedMessage).toEqual(
      expect.objectContaining({
        type: 'terminal.prompt_response',
        payload: {
          workflowId: 'wf-arrow-select-flow',
          terminalId: 'term-select',
          response: '2\n',
        },
      })
    );

    ws!.simulateMessage({
      type: 'terminal.completed',
      payload: {
        workflow_id: 'wf-arrow-select-flow',
        task_id: 'task-select',
        terminal_id: 'term-select',
        status: 'review_passed',
      },
      timestamp: new Date().toISOString(),
      id: 'msg-select-completed',
    });

    expect(onCompleted).toHaveBeenCalledWith(
      expect.objectContaining({
        workflowId: 'wf-arrow-select-flow',
        taskId: 'task-select',
        terminalId: 'term-select',
        status: 'review_pass',
      })
    );
    expect(onCompleted.mock.invocationCallOrder[0]).toBeGreaterThan(
      ws!.send.mock.invocationCallOrder[0]
    );
  });

  it('sendPromptResponse sends terminal.prompt_response with generated message envelope', async () => {
    const { connectToWorkflow, sendPromptResponse } = useWsStore.getState();

    connectToWorkflow('wf-prompt-send');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-prompt-send')
      ).toBe('connected');
    });

    const ws = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-prompt-send/')
    );
    expect(ws).toBeDefined();

    ws!.send.mockClear();

    sendPromptResponse({
      workflowId: 'wf-prompt-send',
      terminalId: 'term-send',
      response: 'y\n',
    });

    expect(ws!.send).toHaveBeenCalledTimes(1);

    const [serializedMessage] = ws!.send.mock.calls[0] as [string];
    const parsedMessage = JSON.parse(serializedMessage) as {
      type: string;
      payload: {
        workflowId: string;
        terminalId: string;
        response: string;
      };
      timestamp: string;
      id: string;
    };

    expect(parsedMessage.type).toBe('terminal.prompt_response');
    expect(parsedMessage.payload).toEqual({
      workflowId: 'wf-prompt-send',
      terminalId: 'term-send',
      response: 'y\n',
    });
    expect(parsedMessage.id).toMatch(/^msg-/);
    expect(Number.isNaN(Date.parse(parsedMessage.timestamp))).toBe(false);
  });

  it('sendPromptResponse does not fall back to another workflow when target is unavailable', async () => {
    const { connectToWorkflow, sendPromptResponse } = useWsStore.getState();

    connectToWorkflow('wf-prompt-send-a');
    connectToWorkflow('wf-prompt-send-b');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-prompt-send-a')
      ).toBe('connected');
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-prompt-send-b')
      ).toBe('connected');
    });

    const wsA = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-prompt-send-a/')
    );
    const wsB = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-prompt-send-b/')
    );

    expect(wsA).toBeDefined();
    expect(wsB).toBeDefined();

    wsA!.send.mockClear();
    wsB!.send.mockClear();
    wsA!.readyState = MockWebSocket.CLOSED;

    const sent = sendPromptResponse({
      workflowId: 'wf-prompt-send-a',
      terminalId: 'term-a',
      response: 'n\n',
    });
    expect(sent).toBe(false);

    expect(wsA!.send).not.toHaveBeenCalled();
    expect(wsB!.send).not.toHaveBeenCalled();
  });

  it('send routes by workflowId payload and never falls back to another workflow', async () => {
    const { connectToWorkflow, send } = useWsStore.getState();

    connectToWorkflow('wf-send-a');
    connectToWorkflow('wf-send-b');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-send-a')
      ).toBe('connected');
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-send-b')
      ).toBe('connected');
    });

    const wsA = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-send-a/')
    );
    const wsB = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-send-b/')
    );

    expect(wsA).toBeDefined();
    expect(wsB).toBeDefined();

    wsA!.send.mockClear();
    wsB!.send.mockClear();
    wsA!.readyState = MockWebSocket.CLOSED;

    const sent = send({
      type: 'terminal.input',
      payload: { workflowId: 'wf-send-a', terminalId: 'term-a', data: 'hello' },
      timestamp: new Date().toISOString(),
      id: 'msg-send-by-workflow',
    });
    expect(sent).toBe(false);

    expect(wsA!.send).not.toHaveBeenCalled();
    expect(wsB!.send).not.toHaveBeenCalled();
  });

  it('send routes to current workflow when payload does not include workflowId', async () => {
    const { connectToWorkflow, send } = useWsStore.getState();

    connectToWorkflow('wf-send-current-a');
    connectToWorkflow('wf-send-current-b');

    await vi.waitFor(() => {
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-send-current-a')
      ).toBe('connected');
      expect(
        useWsStore.getState().getWorkflowConnectionStatus('wf-send-current-b')
      ).toBe('connected');
    });

    const wsA = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-send-current-a/')
    );
    const wsB = websocketInstances.find((instance) =>
      instance.url.includes('/workflow/wf-send-current-b/')
    );

    expect(wsA).toBeDefined();
    expect(wsB).toBeDefined();

    wsA!.send.mockClear();
    wsB!.send.mockClear();

    const sent = send({
      type: 'system.heartbeat',
      payload: {},
      timestamp: new Date().toISOString(),
      id: 'msg-send-current',
    });
    expect(sent).toBe(true);

    expect(wsA!.send).not.toHaveBeenCalled();
    expect(wsB!.send).toHaveBeenCalledTimes(1);
  });
});
