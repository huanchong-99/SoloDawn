import { create } from 'zustand';
import type {
  WizardConfig,
  TaskConfig,
  ModelConfig,
  TerminalConfig,
  ProjectConfig,
  BasicConfig,
  CommandConfig,
  AdvancedConfig,
} from '@/components/workflow/types';
import { getDefaultWizardConfig } from '@/components/workflow/types';

/**
 * Wizard state management store
 * Manages multi-step workflow creation wizard
 */
interface WizardStoreState {
  // State
  currentStep: number;
  config: WizardConfig;
  errors: Record<string, string>;
  isDirty: boolean;
  isSubmitting: boolean;

  // Navigation
  setStep: (step: number) => void;
  nextStep: () => void;
  prevStep: () => void;
  canGoNext: () => boolean;
  canGoPrev: () => boolean;

  // Config updates
  updateProject: (updates: Partial<ProjectConfig>) => void;
  updateBasic: (updates: Partial<BasicConfig>) => void;
  updateCommands: (updates: Partial<CommandConfig>) => void;
  updateAdvanced: (updates: Partial<AdvancedConfig>) => void;
  setConfig: (config: WizardConfig) => void;

  // Task management
  setTasks: (tasks: TaskConfig[]) => void;
  updateTask: (index: number, updates: Partial<TaskConfig>) => void;
  addTask: (task: TaskConfig) => void;
  removeTask: (index: number) => void;

  // Model management
  setModels: (models: ModelConfig[]) => void;
  addModel: (model: ModelConfig) => void;
  updateModel: (id: string, updates: Partial<ModelConfig>) => void;
  removeModel: (id: string) => void;

  // Terminal management
  setTerminals: (terminals: TerminalConfig[]) => void;
  addTerminal: (terminal: TerminalConfig) => void;
  updateTerminal: (id: string, updates: Partial<TerminalConfig>) => void;
  removeTerminal: (id: string) => void;

  // Validation
  setErrors: (errors: Record<string, string>) => void;
  setError: (key: string, message: string) => void;
  clearError: (key: string) => void;
  clearAllErrors: () => void;
  hasErrors: () => boolean;

  // Submission
  setSubmitting: (submitting: boolean) => void;

  // Reset
  reset: () => void;
}

const TOTAL_STEPS = 7; // Steps 0-6

export const useWizardStore = create<WizardStoreState>((set, get) => ({
  // Initial state
  currentStep: 0,
  config: getDefaultWizardConfig(),
  errors: {},
  isDirty: false,
  isSubmitting: false,

  // Navigation
  setStep: (step) => {
    if (step >= 0 && step < TOTAL_STEPS) {
      set({ currentStep: step });
    }
  },

  nextStep: () => {
    const { currentStep } = get();
    if (currentStep < TOTAL_STEPS - 1) {
      set({ currentStep: currentStep + 1 });
    }
  },

  prevStep: () => {
    const { currentStep } = get();
    if (currentStep > 0) {
      set({ currentStep: currentStep - 1 });
    }
  },

  canGoNext: () => {
    const { currentStep, errors } = get();
    return currentStep < TOTAL_STEPS - 1 && Object.keys(errors).length === 0;
  },

  canGoPrev: () => {
    return get().currentStep > 0;
  },

  // Config updates
  updateProject: (updates) => {
    set((state) => ({
      config: {
        ...state.config,
        project: { ...state.config.project, ...updates },
      },
      isDirty: true,
    }));
  },

  updateBasic: (updates) => {
    set((state) => ({
      config: {
        ...state.config,
        basic: { ...state.config.basic, ...updates },
      },
      isDirty: true,
    }));
  },

  updateCommands: (updates) => {
    set((state) => ({
      config: {
        ...state.config,
        commands: { ...state.config.commands, ...updates },
      },
      isDirty: true,
    }));
  },

  updateAdvanced: (updates) => {
    set((state) => ({
      config: {
        ...state.config,
        advanced: { ...state.config.advanced, ...updates },
      },
      isDirty: true,
    }));
  },

  setConfig: (config) => {
    set({ config, isDirty: true });
  },

  // Task management
  setTasks: (tasks) => {
    set((state) => ({
      config: { ...state.config, tasks },
      isDirty: true,
    }));
  },

  updateTask: (index, updates) => {
    set((state) => {
      const tasks = [...state.config.tasks];
      if (index >= 0 && index < tasks.length) {
        tasks[index] = { ...tasks[index], ...updates };
      }
      return {
        config: { ...state.config, tasks },
        isDirty: true,
      };
    });
  },

  addTask: (task) => {
    set((state) => ({
      config: {
        ...state.config,
        tasks: [...state.config.tasks, task],
      },
      isDirty: true,
    }));
  },

  removeTask: (index) => {
    set((state) => {
      const tasks = state.config.tasks.filter((_, i) => i !== index);
      return {
        config: { ...state.config, tasks },
        isDirty: true,
      };
    });
  },

  // Model management
  setModels: (models) => {
    set((state) => ({
      config: { ...state.config, models },
      isDirty: true,
    }));
  },

  addModel: (model) => {
    set((state) => ({
      config: {
        ...state.config,
        models: [...state.config.models, model],
      },
      isDirty: true,
    }));
  },

  updateModel: (id, updates) => {
    set((state) => {
      const models = state.config.models.map((m) =>
        m.id === id ? { ...m, ...updates } : m
      );
      return {
        config: { ...state.config, models },
        isDirty: true,
      };
    });
  },

  removeModel: (id) => {
    set((state) => {
      const models = state.config.models.filter((m) => m.id !== id);
      // Clear orchestrator model if it was removed
      const advanced = { ...state.config.advanced };
      if (advanced.orchestrator.modelConfigId === id) {
        advanced.orchestrator = { ...advanced.orchestrator, modelConfigId: '' };
      }
      return {
        config: { ...state.config, models, advanced },
        isDirty: true,
      };
    });
  },

  // Terminal management
  setTerminals: (terminals) => {
    set((state) => ({
      config: { ...state.config, terminals },
      isDirty: true,
    }));
  },

  addTerminal: (terminal) => {
    set((state) => ({
      config: {
        ...state.config,
        terminals: [...state.config.terminals, terminal],
      },
      isDirty: true,
    }));
  },

  updateTerminal: (id, updates) => {
    set((state) => {
      const terminals = state.config.terminals.map((t) =>
        t.id === id ? { ...t, ...updates } : t
      );
      return {
        config: { ...state.config, terminals },
        isDirty: true,
      };
    });
  },

  removeTerminal: (id) => {
    set((state) => {
      const terminals = state.config.terminals.filter((t) => t.id !== id);
      return {
        config: { ...state.config, terminals },
        isDirty: true,
      };
    });
  },

  // Validation
  setErrors: (errors) => {
    set({ errors });
  },

  setError: (key, message) => {
    set((state) => ({
      errors: { ...state.errors, [key]: message },
    }));
  },

  clearError: (key) => {
    set((state) => {
      const { [key]: _removed, ...rest } = state.errors;
      void _removed;
      return { errors: rest };
    });
  },

  clearAllErrors: () => {
    set({ errors: {} });
  },

  hasErrors: () => {
    return Object.keys(get().errors).length > 0;
  },

  // Submission
  setSubmitting: (submitting) => {
    set({ isSubmitting: submitting });
  },

  // Reset
  reset: () => {
    set({
      currentStep: 0,
      config: getDefaultWizardConfig(),
      errors: {},
      isDirty: false,
      isSubmitting: false,
    });
  },
}));

/**
 * Hook to get current step config
 */
export function useCurrentStepConfig() {
  const currentStep = useWizardStore((state) => state.currentStep);
  const config = useWizardStore((state) => state.config);
  return { currentStep, config };
}

/**
 * Hook to check if wizard has unsaved changes
 */
export function useWizardDirty() {
  return useWizardStore((state) => state.isDirty);
}
