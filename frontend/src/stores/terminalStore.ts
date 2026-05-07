import { create } from 'zustand';

/**
 * Terminal state for a single terminal instance
 */
export interface TerminalState {
  id: string;
  workflowTaskId: string;
  status: string;
  output: string[];
  maxOutputLines: number;
  isConnected: boolean;
  lastActivity: Date | null;
}

/**
 * Terminal state management store
 * Manages terminal connections, output buffers, and status
 */
interface TerminalStoreState {
  // State
  terminals: Map<string, TerminalState>;
  activeTerminalId: string | null;

  // Actions
  setActiveTerminal: (id: string | null) => void;
  initTerminal: (id: string, workflowTaskId: string) => void;
  removeTerminal: (id: string) => void;
  appendOutput: (terminalId: string, data: string) => void;
  clearOutput: (terminalId: string) => void;
  updateStatus: (terminalId: string, status: string) => void;
  setConnected: (terminalId: string, connected: boolean) => void;
  reset: () => void;

  // Selectors
  getTerminal: (id: string) => TerminalState | undefined;
  getActiveTerminal: () => TerminalState | undefined;
  getTerminalOutput: (id: string) => string[];
  getTerminalsByTask: (workflowTaskId: string) => TerminalState[];
}

const DEFAULT_MAX_OUTPUT_LINES = 10000;

const createDefaultTerminalState = (id: string, workflowTaskId: string): TerminalState => ({
  id,
  workflowTaskId,
  status: 'idle',
  output: [],
  maxOutputLines: DEFAULT_MAX_OUTPUT_LINES,
  isConnected: false,
  lastActivity: null,
});

export const useTerminalStore = create<TerminalStoreState>((set, get) => ({
  // Initial state
  terminals: new Map(),
  activeTerminalId: null,

  setActiveTerminal: (id) => {
    set({ activeTerminalId: id });
  },

  initTerminal: (id, workflowTaskId) => {
    set((state) => {
      // Don't reinitialize if already exists
      if (state.terminals.has(id)) {
        return state;
      }

      const newTerminals = new Map(state.terminals);
      newTerminals.set(id, createDefaultTerminalState(id, workflowTaskId));
      return { terminals: newTerminals };
    });
  },

  removeTerminal: (id) => {
    set((state) => {
      const newTerminals = new Map(state.terminals);
      newTerminals.delete(id);

      // Clear active terminal if it was removed
      const newActiveId = state.activeTerminalId === id ? null : state.activeTerminalId;

      return { terminals: newTerminals, activeTerminalId: newActiveId };
    });
  },

  appendOutput: (terminalId, data) => {
    set((state) => {
      const terminal = state.terminals.get(terminalId);
      if (!terminal) return state;

      // Split data by newlines (handle both \n and \r\n)
      const newLines = data.split(/\r?\n/);
      let updatedOutput = [...terminal.output, ...newLines];

      // Trim output if exceeds max lines
      if (updatedOutput.length > terminal.maxOutputLines) {
        updatedOutput = updatedOutput.slice(-terminal.maxOutputLines);
      }

      const newTerminals = new Map(state.terminals);
      newTerminals.set(terminalId, {
        ...terminal,
        output: updatedOutput,
        lastActivity: new Date(),
      });

      return { terminals: newTerminals };
    });
  },

  clearOutput: (terminalId) => {
    set((state) => {
      const terminal = state.terminals.get(terminalId);
      if (!terminal) return state;

      const newTerminals = new Map(state.terminals);
      newTerminals.set(terminalId, {
        ...terminal,
        output: [],
      });

      return { terminals: newTerminals };
    });
  },

  updateStatus: (terminalId, status) => {
    set((state) => {
      const terminal = state.terminals.get(terminalId);
      if (!terminal) return state;

      const newTerminals = new Map(state.terminals);
      newTerminals.set(terminalId, {
        ...terminal,
        status,
        lastActivity: new Date(),
      });

      return { terminals: newTerminals };
    });
  },

  setConnected: (terminalId, connected) => {
    set((state) => {
      const terminal = state.terminals.get(terminalId);
      if (!terminal) return state;

      const newTerminals = new Map(state.terminals);
      newTerminals.set(terminalId, {
        ...terminal,
        isConnected: connected,
        lastActivity: new Date(),
      });

      return { terminals: newTerminals };
    });
  },

  reset: () => {
    set({
      terminals: new Map(),
      activeTerminalId: null,
    });
  },

  // Selectors
  getTerminal: (id) => {
    return get().terminals.get(id);
  },

  getActiveTerminal: () => {
    const { terminals, activeTerminalId } = get();
    return activeTerminalId ? terminals.get(activeTerminalId) : undefined;
  },

  getTerminalOutput: (id) => {
    const terminal = get().terminals.get(id);
    return terminal?.output ?? [];
  },

  getTerminalsByTask: (workflowTaskId) => {
    const terminals = Array.from(get().terminals.values());
    return terminals.filter((t) => t.workflowTaskId === workflowTaskId);
  },
}));

/**
 * Hook to get terminal output as a single string
 */
export function useTerminalOutputString(terminalId: string): string {
  const output = useTerminalStore((state) => state.terminals.get(terminalId)?.output ?? []);
  return output.join('\n');
}

/**
 * Hook to get active terminal
 *
 * W2-22-10: Select the resolved active terminal directly from store state so
 * we only re-render when the active terminal itself changes. Previously we
 * subscribed to the full `terminals` Map, causing re-renders for any terminal
 * mutation even when the active terminal was unchanged.
 */
export function useActiveTerminal(): TerminalState | undefined {
  return useTerminalStore((state) =>
    state.activeTerminalId ? state.terminals.get(state.activeTerminalId) : undefined
  );
}

/**
 * Hook to get recent output lines (for activity panel)
 */
export function useRecentTerminalOutput(terminalId: string, lineCount = 5): string[] {
  const output = useTerminalStore((state) => state.terminals.get(terminalId)?.output ?? []);
  return output.slice(-lineCount);
}
