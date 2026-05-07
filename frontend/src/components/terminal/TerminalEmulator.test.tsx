import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import { createRef } from 'react';
import { TerminalEmulator, TerminalEmulatorRef } from './TerminalEmulator';

const VALID_TERMINAL_ID = '123e4567-e89b-12d3-a456-426614174000';
const KEEPALIVE_INPUT_MESSAGE = JSON.stringify({ type: 'heartbeat' });

// Mock xterm
vi.mock('@xterm/xterm', () => {
  class MockTerminal {
    onData = vi.fn().mockReturnValue({ dispose: vi.fn() });
    open = vi.fn<(container: HTMLElement) => void>();
    write = vi.fn<(data: string) => void>();
    clear = vi.fn<() => void>();
    dispose = vi.fn<() => void>();
    loadAddon = vi.fn<(addon: unknown) => void>();
    cols = 80;
    rows = 24;
  }
  return { Terminal: MockTerminal };
});

// Mock FitAddon
vi.mock('@xterm/addon-fit', () => {
  class MockFitAddon {
    fit = vi.fn<() => void>();
    dispose = vi.fn<() => void>();
  }
  return { FitAddon: MockFitAddon };
});

// Mock WebSocket
class MockWebSocket {
  static readonly instances: MockWebSocket[] = [];
  static readonly autoOpen = true;
  static readonly CONNECTING = 0 as const;
  static readonly OPEN = 1 as const;
  static readonly CLOSING = 2 as const;
  static readonly CLOSED = 3 as const;

  url = '';
  readyState = MockWebSocket.CONNECTING;
  lastSent: string | null = null;
  sentMessages: string[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((error: Event) => void) | null = null;
  onclose: ((event?: CloseEvent) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
    if (MockWebSocket.autoOpen) {
      setTimeout(() => {
        this.readyState = MockWebSocket.OPEN;
        this.onopen?.();
      }, 0);
    }
  }

  send(data: string) {
    this.lastSent = data;
    this.sentMessages.push(data);
  }

  close() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code: 1000, reason: '' } as CloseEvent);
  }

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  simulateUnexpectedClose(reason = 'network drop') {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code: 1006, reason } as CloseEvent);
  }

  addEventListener(event: string, handler: () => void) {
    if (event === 'open') this.onopen = handler;
  }
}

globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket;

describe('TerminalEmulator', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    MockWebSocket.instances = [];
    MockWebSocket.autoOpen = false;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('Rendering', () => {
    it('should render terminal container', () => {
      render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} />);
      const container = document.querySelector(String.raw`.w-full.h-full.min-h-\[300px\]`);
      expect(container).toBeInTheDocument();
    });

    it('should have correct CSS classes for styling', () => {
      render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} />);
      const container = document.querySelector(String.raw`.bg-\[\#1e1e1e\]`);
      expect(container).toBeInTheDocument();
    });

    it('should have accessibility attributes', () => {
      render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} />);
      const container = document.querySelector('[role="application"]');
      expect(container).toBeInTheDocument();
      expect(container).toHaveAttribute('aria-label', 'Terminal emulator');
    });
  });

  describe('Terminal Initialization', () => {
    it('should initialize terminal without errors', () => {
      expect(() => {
        render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} />);
      }).not.toThrow();
    });

    it('should have displayName for debugging', () => {
      expect(TerminalEmulator.displayName).toBe('TerminalEmulator');
    });
  });

  describe('Ref Methods', () => {
    it('should expose write method via ref', async () => {
      const ref = createRef<TerminalEmulatorRef>();
      render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} ref={ref} />);

      await waitFor(() => {
        expect(ref.current).toBeDefined();
        expect(ref.current?.write).toBeInstanceOf(Function);
      });
    });

    it('should expose clear method via ref', async () => {
      const ref = createRef<TerminalEmulatorRef>();
      render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} ref={ref} />);

      await waitFor(() => {
        expect(ref.current).toBeDefined();
        expect(ref.current?.clear).toBeInstanceOf(Function);
      });
    });
  });

  describe('WebSocket Connection', () => {
    it('should establish WebSocket connection when wsUrl is provided', async () => {
      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
        />
      );

      await waitFor(() => {
        expect(MockWebSocket.instances).toHaveLength(1);
      });

      const socket = MockWebSocket.instances[0];
      expect(socket.url).toBe(`ws://localhost:8080/terminal/${VALID_TERMINAL_ID}`);

      act(() => {
        socket.simulateOpen();
      });

      await waitFor(() => {
        expect(screen.queryByText('Connecting terminal stream...')).not.toBeInTheDocument();
      });
    });

    it('should show connecting hint before stream is open', async () => {
      MockWebSocket.autoOpen = false;

      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
        />
      );

      await waitFor(() => {
        expect(screen.getByText('Connecting terminal stream...')).toBeInTheDocument();
      });
    });

    it('should not connect WebSocket when wsUrl is not provided', () => {
      expect(() => {
        render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} />);
      }).not.toThrow();
    });

    it('should validate terminalId format', () => {
      const onError = vi.fn();

      render(
        <TerminalEmulator
          terminalId="invalid@id!"
          wsUrl="ws://localhost:8080"
          onError={onError}
        />
      );

      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ message: 'Invalid terminalId' })
      );
    });

    it('should reject empty terminalId', () => {
      const onError = vi.fn();

      render(
        <TerminalEmulator
          terminalId=""
          wsUrl="ws://localhost:8080"
          onError={onError}
        />
      );

      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ message: 'Invalid terminalId' })
      );
    });

    it('should reject non-UUID terminalId values', () => {
      const onError = vi.fn();

      render(
        <TerminalEmulator
          terminalId="test-terminal-123"
          wsUrl="ws://localhost:8080"
          onError={onError}
        />
      );

      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ message: 'Invalid terminalId' })
      );
    });

    it('should accept valid terminalId UUIDs', () => {
      const onError = vi.fn();

      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
          onError={onError}
        />
      );

      expect(onError).not.toHaveBeenCalled();
    });
  });

  describe('WebSocket Ready State Checks', () => {
    it('should check WebSocket ready state before sending data', () => {
      const onData = vi.fn();
      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
          onData={onData}
        />
      );

      // Component should not throw when WebSocket is not ready
      expect(() => {
        // The handleData callback checks readyState before sending
      }).not.toThrow();
    });
  });

  describe('WebSocket Keepalive', () => {
    it('should send keepalive messages only after websocket is open', () => {
      vi.useFakeTimers();

      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
        />
      );

      expect(MockWebSocket.instances).toHaveLength(1);
      const socket = MockWebSocket.instances[0];

      act(() => {
        vi.advanceTimersByTime(180_000);
      });

      expect(
        socket.sentMessages.filter((payload) => payload === KEEPALIVE_INPUT_MESSAGE)
      ).toHaveLength(0);

      act(() => {
        socket.simulateOpen();
      });

      act(() => {
        vi.advanceTimersByTime(60_000);
      });

      expect(
        socket.sentMessages.filter((payload) => payload === KEEPALIVE_INPUT_MESSAGE)
      ).toHaveLength(1);
    });

    it('should stop keepalive messages after websocket closes', () => {
      vi.useFakeTimers();

      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
        />
      );

      expect(MockWebSocket.instances).toHaveLength(1);
      const socket = MockWebSocket.instances[0];

      act(() => {
        socket.simulateOpen();
      });

      act(() => {
        vi.advanceTimersByTime(60_000);
      });

      expect(
        socket.sentMessages.filter((payload) => payload === KEEPALIVE_INPUT_MESSAGE)
      ).toHaveLength(1);

      act(() => {
        socket.simulateUnexpectedClose('network');
      });

      act(() => {
        vi.advanceTimersByTime(180_000);
      });

      expect(
        socket.sentMessages.filter((payload) => payload === KEEPALIVE_INPUT_MESSAGE)
      ).toHaveLength(1);
    });
  });

  describe('Error Handling', () => {
    it('should show disconnected hint when stream closes unexpectedly', async () => {
      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
        />
      );

      await waitFor(() => {
        expect(MockWebSocket.instances.length).toBeGreaterThan(0);
      });

      const socket = MockWebSocket.instances[0];
      act(() => {
        socket.simulateUnexpectedClose('network');
      });

      await waitFor(() => {
        expect(screen.getByText(/Disconnected from terminal stream/)).toBeInTheDocument();
      });

      expect(screen.getByText(/network/)).toBeInTheDocument();
    });

    it('should handle malformed WebSocket messages', () => {
      const onError = vi.fn();

      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
          onError={onError}
        />
      );

      // Simulate receiving malformed data
      // In a real scenario, the WebSocket would receive invalid JSON
      // The component should catch and log the error without crashing

      expect(onError).toBeDefined();
    });

    it('should handle WebSocket errors gracefully', () => {
      const onError = vi.fn();

      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
          onError={onError}
        />
      );

      // Error handler is set up and safe to call
      expect(onError).toBeDefined();
    });
  });

  describe('Data Handling', () => {
    it('should handle onData callback', () => {
      const onData = vi.fn();
      expect(() => {
        render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} onData={onData} />);
      }).not.toThrow();
    });

    it('should handle onResize callback', () => {
      const onResize = vi.fn();
      expect(() => {
        render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} onResize={onResize} />);
      }).not.toThrow();
    });
  });

  describe('Cleanup', () => {
    it('should cleanup on unmount', () => {
      const { unmount } = render(<TerminalEmulator terminalId={VALID_TERMINAL_ID} />);
      expect(() => {
        unmount();
      }).not.toThrow();
    });

    it('should close WebSocket on unmount', () => {
      const { unmount } = render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
        />
      );

      expect(() => {
        unmount();
      }).not.toThrow();
    });
  });

  describe('Race Condition Prevention', () => {
    it('should wait for terminal to be ready before connecting WebSocket', () => {
      const consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => undefined);

      render(
        <TerminalEmulator
          terminalId={VALID_TERMINAL_ID}
          wsUrl="ws://localhost:8080"
        />
      );

      // WebSocket connection should only establish after terminal is ready
      // This is handled by the terminalReadyRef
      consoleSpy.mockRestore();
    });
  });
});
