import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, fireEvent } from '@testing-library/react';
import { Step6Advanced } from './Step6Advanced';
import type { WizardConfig } from '../types';
import { renderWithI18n, setTestLanguage, i18n } from '@/test/renderWithI18n';

describe('Step6Advanced', () => {
  const mockOnUpdate = vi.fn<(updates: Partial<WizardConfig>) => void>();
  const getLastUpdate = (): Partial<WizardConfig> => {
    const calls = mockOnUpdate.mock.calls;
    expect(calls.length).toBeGreaterThan(0);
    return calls[calls.length - 1][0];
  };
  const getLastAdvanced = () => {
    const lastCall = getLastUpdate();
    if (!lastCall.advanced) {
      throw new Error('Expected advanced config to be defined.');
    }
    return lastCall.advanced;
  };

  const defaultConfig: WizardConfig = {
    project: {
      workingDirectory: '/test',
      gitStatus: { isGitRepo: true, isDirty: false },
    },
    basic: {
      name: 'Test Workflow',
      taskCount: 1,
    },
    tasks: [],
    models: [],
    terminals: [],
    commands: {
      enabled: false,
      presetIds: [],
    },
    advanced: {
      orchestrator: { modelConfigId: '' },
      errorTerminal: { enabled: false },
      mergeTerminal: {
        cliTypeId: '',
        modelConfigId: '',
        runTestsBeforeMerge: true,
        pauseOnConflict: true,
      },
      targetBranch: 'main',
    },
  };

  const configWithModels: WizardConfig = {
    ...defaultConfig,
    models: [
      {
        id: 'model-1',
        displayName: 'Claude 3.5',
        apiType: 'anthropic',
        baseUrl: 'https://api.anthropic.com',
        apiKey: 'sk-test',
        modelId: 'claude-3-5-sonnet-20241022',
        isVerified: true,
      },
      {
        id: 'model-2',
        displayName: 'GPT-4',
        apiType: 'openai',
        baseUrl: 'https://api.openai.com',
        apiKey: 'sk-test2',
        modelId: 'gpt-4',
        isVerified: true,
      },
    ],
  };

  beforeEach(() => {
    mockOnUpdate.mockClear();
    void setTestLanguage();
  });

  describe('Orchestrator Configuration', () => {
    it('should render orchestrator model selection', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const label = i18n.t('workflow:step6.orchestrator.label');
      expect(screen.getByText(label)).toBeInTheDocument();
      expect(screen.getByLabelText(label)).toBeInTheDocument();
    });

    it('should display available models in orchestrator dropdown', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.orchestrator.label'));
      expect(select).toBeInTheDocument();

      const options = screen.getAllByText(/Claude 3\.5|GPT-4/);
      expect(options.length).toBeGreaterThan(0);
    });

    it('should update orchestrator model when selection changes', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.orchestrator.label'));
      fireEvent.change(select, { target: { value: 'model-1' } });

      const advanced = getLastAdvanced();
      expect(advanced.orchestrator.modelConfigId).toBe('model-1');
    });

    it('should show validation error for orchestrator model', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{ orchestratorModel: 'validation.advanced.orchestratorModelRequired' }}
        />
      );

      expect(
        screen.getByText(i18n.t('workflow:validation.advanced.orchestratorModelRequired'))
      ).toBeInTheDocument();
    });
  });

  describe('Error Terminal Configuration', () => {
    it('should render error terminal toggle', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      expect(
        screen.getByLabelText(i18n.t('workflow:step6.errorTerminal.enableLabel'))
      ).toBeInTheDocument();
    });

    it('should not show error terminal options when disabled', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      expect(
        screen.queryByLabelText(i18n.t('workflow:step6.errorTerminal.cliLabel'))
      ).not.toBeInTheDocument();
      expect(
        screen.queryByLabelText(i18n.t('workflow:step6.errorTerminal.modelLabel'))
      ).not.toBeInTheDocument();
    });

    it('should show error terminal options when enabled', () => {
      const configWithErrorTerminalEnabled: WizardConfig = {
        ...defaultConfig,
        advanced: {
          ...defaultConfig.advanced,
          errorTerminal: { enabled: true },
        },
      };

      renderWithI18n(
        <Step6Advanced
          config={configWithErrorTerminalEnabled}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      expect(
        screen.getByLabelText(i18n.t('workflow:step6.errorTerminal.cliLabel'))
      ).toBeInTheDocument();
      expect(
        screen.getByLabelText(i18n.t('workflow:step6.errorTerminal.modelLabel'))
      ).toBeInTheDocument();
    });

    it('should enable error terminal when checkbox is checked', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const checkbox = screen.getByLabelText(
        i18n.t('workflow:step6.errorTerminal.enableLabel')
      );
      fireEvent.click(checkbox);

      const advanced = getLastAdvanced();
      expect(advanced.errorTerminal.enabled).toBe(true);
    });

    it('should clear error terminal config when disabled', () => {
      const configWithErrorTerminalEnabled: WizardConfig = {
        ...defaultConfig,
        advanced: {
          ...defaultConfig.advanced,
          errorTerminal: {
            enabled: true,
            cliTypeId: 'cli-claude-code',
            modelConfigId: 'model-1',
          },
        },
      };

      renderWithI18n(
        <Step6Advanced
          config={configWithErrorTerminalEnabled}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const checkbox = screen.getByLabelText(
        i18n.t('workflow:step6.errorTerminal.enableLabel')
      );
      fireEvent.click(checkbox);

      const advanced = getLastAdvanced();
      expect(advanced.errorTerminal.enabled).toBe(false);
      expect(advanced.errorTerminal.cliTypeId).toBeUndefined();
      expect(advanced.errorTerminal.modelConfigId).toBeUndefined();
    });

    it('should update error terminal CLI selection', () => {
      const configWithErrorTerminalEnabled: WizardConfig = {
        ...configWithModels,
        advanced: {
          ...configWithModels.advanced,
          errorTerminal: { enabled: true, cliTypeId: 'cli-claude-code' },
        },
      };

      renderWithI18n(
        <Step6Advanced
          config={configWithErrorTerminalEnabled}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.errorTerminal.cliLabel'));
      fireEvent.change(select, { target: { value: 'cli-claude-code' } });

      const advanced = getLastAdvanced();
      expect(advanced.errorTerminal.cliTypeId).toBe('cli-claude-code');
    });

    it('should update error terminal model selection', () => {
      const configWithErrorTerminalEnabled: WizardConfig = {
        ...configWithModels,
        advanced: {
          ...configWithModels.advanced,
          errorTerminal: { enabled: true, cliTypeId: 'cli-claude-code' },
        },
      };

      renderWithI18n(
        <Step6Advanced
          config={configWithErrorTerminalEnabled}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.errorTerminal.modelLabel'));
      fireEvent.change(select, { target: { value: 'model-1' } });

      const advanced = getLastAdvanced();
      expect(advanced.errorTerminal.modelConfigId).toBe('model-1');
    });

    it('should show validation errors for error terminal fields', () => {
      const configWithErrorTerminalEnabled: WizardConfig = {
        ...configWithModels,
        advanced: {
          ...configWithModels.advanced,
          errorTerminal: { enabled: true },
        },
      };

      renderWithI18n(
        <Step6Advanced
          config={configWithErrorTerminalEnabled}
          onUpdate={mockOnUpdate}
          errors={{
            errorTerminalCli: 'validation.terminals.cliRequired',
            errorTerminalModel: 'validation.terminals.modelRequired',
          }}
        />
      );

      expect(
        screen.getAllByText(i18n.t('workflow:validation.terminals.cliRequired')).length
      ).toBeGreaterThanOrEqual(1);
      expect(
        screen.getAllByText(i18n.t('workflow:validation.terminals.modelRequired')).length
      ).toBeGreaterThanOrEqual(1);
    });
  });

  describe('Merge Terminal Configuration', () => {
    it('should render merge terminal configuration', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      expect(screen.getByText(i18n.t('workflow:step6.mergeTerminal.title'))).toBeInTheDocument();
      expect(screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.cliLabel'))).toBeInTheDocument();
      expect(screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.modelLabel'))).toBeInTheDocument();
    });

    it('should update merge terminal CLI selection', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.cliLabel'));
      fireEvent.change(select, { target: { value: 'cli-gemini-cli' } });

      const advanced = getLastAdvanced();
      expect(advanced.mergeTerminal.cliTypeId).toBe('cli-gemini-cli');
    });

    it('should update merge terminal model selection', () => {
      renderWithI18n(
        <Step6Advanced
          config={{
            ...configWithModels,
            advanced: {
              ...configWithModels.advanced,
              mergeTerminal: {
                ...configWithModels.advanced.mergeTerminal,
                cliTypeId: 'cli-claude-code',
              },
            },
          }}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.modelLabel'));
      fireEvent.change(select, { target: { value: 'model-2' } });

      const advanced = getLastAdvanced();
      expect(advanced.mergeTerminal.modelConfigId).toBe('model-2');
    });

    it('should have runTestsBeforeMerge checked by default', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const checkbox = screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.runTestsLabel'));
      expect(checkbox).toBeChecked();
    });

    it('should have pauseOnConflict checked by default', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const checkbox = screen.getByLabelText(
        i18n.t('workflow:step6.mergeTerminal.pauseOnConflictLabel')
      );
      expect(checkbox).toBeChecked();
    });

    it('should update runTestsBeforeMerge when checkbox is toggled', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const checkbox = screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.runTestsLabel'));
      fireEvent.click(checkbox);

      const advanced = getLastAdvanced();
      expect(advanced.mergeTerminal.runTestsBeforeMerge).toBe(false);
    });

    it('should update pauseOnConflict when checkbox is toggled', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const checkbox = screen.getByLabelText(
        i18n.t('workflow:step6.mergeTerminal.pauseOnConflictLabel')
      );
      fireEvent.click(checkbox);

      const advanced = getLastAdvanced();
      expect(advanced.mergeTerminal.pauseOnConflict).toBe(false);
    });

    it('should show validation errors for merge terminal fields', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{
            mergeCli: 'validation.advanced.mergeCliRequired',
            mergeModel: 'validation.advanced.mergeModelRequired',
          }}
        />
      );

      expect(
        screen.getAllByText(i18n.t('workflow:validation.advanced.mergeCliRequired')).length
      ).toBeGreaterThanOrEqual(1);
      expect(
        screen.getAllByText(i18n.t('workflow:validation.advanced.mergeModelRequired')).length
      ).toBeGreaterThanOrEqual(1);
    });
  });

  describe('Target Branch', () => {
    it('should render target branch input', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      expect(
        screen.getByLabelText(i18n.t('workflow:step6.targetBranch.label'))
      ).toBeInTheDocument();
    });

    it('should display current target branch value', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const input = screen.getByLabelText(i18n.t('workflow:step6.targetBranch.label'));
      expect(input).toHaveValue('main');
    });

    it('should update target branch when input changes', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const input = screen.getByLabelText(i18n.t('workflow:step6.targetBranch.label'));
      fireEvent.change(input, { target: { value: 'develop' } });

      const advanced = getLastAdvanced();
      expect(advanced.targetBranch).toBe('develop');
    });

    it('should show validation error for target branch', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{ targetBranch: 'validation.advanced.targetBranchRequired' }}
        />
      );

      expect(
        screen.getByText(i18n.t('workflow:validation.advanced.targetBranchRequired'))
      ).toBeInTheDocument();
    });
  });

  describe('Git Commit Format', () => {
    it('should render collapsible Git commit format section', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      expect(
        screen.getByText(i18n.t('workflow:step6.gitCommit.title'))
      ).toBeInTheDocument();
    });

    it('should display Git commit format template', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const toggleButton = screen.getByText(i18n.t('workflow:step6.gitCommit.title')).closest('button');
      if (toggleButton) {
        fireEvent.click(toggleButton);
      }

      expect(screen.getByText(/<type>/)).toBeInTheDocument();
      expect(screen.getByText(/<subject>/)).toBeInTheDocument();
      expect(screen.getByText(/Co-Authored-By:/)).toBeInTheDocument();
    });
  });

  describe('Helper Functions', () => {
    it('should use updateOrchestrator helper function', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.orchestrator.label'));
      fireEvent.change(select, { target: { value: 'model-1' } });

      const calls = mockOnUpdate.mock.calls;
      expect(calls.length).toBeGreaterThan(0);

      const lastCall = calls[calls.length - 1][0];
      expect(lastCall).toHaveProperty('advanced');
      expect(lastCall.advanced).toHaveProperty('orchestrator');
      expect(lastCall.advanced.orchestrator).toHaveProperty('modelConfigId', 'model-1');
    });

    it('should use updateErrorTerminal helper function', () => {
      const configWithErrorTerminalEnabled: WizardConfig = {
        ...configWithModels,
        advanced: {
          ...configWithModels.advanced,
          errorTerminal: { enabled: true },
        },
      };

      renderWithI18n(
        <Step6Advanced
          config={configWithErrorTerminalEnabled}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.errorTerminal.cliLabel'));
      fireEvent.change(select, { target: { value: 'cli-claude-code' } });

      const calls = mockOnUpdate.mock.calls;
      expect(calls.length).toBeGreaterThan(0);

      const lastCall = calls[calls.length - 1][0];
      expect(lastCall).toHaveProperty('advanced');
      expect(lastCall.advanced).toHaveProperty('errorTerminal');
      expect(lastCall.advanced.errorTerminal).toHaveProperty('cliTypeId', 'cli-claude-code');
    });

    it('should use updateMergeTerminal helper function', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const select = screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.cliLabel'));
      fireEvent.change(select, { target: { value: 'cli-codex' } });

      const calls = mockOnUpdate.mock.calls;
      expect(calls.length).toBeGreaterThan(0);

      const lastCall = calls[calls.length - 1][0];
      expect(lastCall).toHaveProperty('advanced');
      expect(lastCall.advanced).toHaveProperty('mergeTerminal');
      expect(lastCall.advanced.mergeTerminal).toHaveProperty('cliTypeId', 'cli-codex');
    });
  });

  describe('Empty Models State', () => {
    it('should show placeholder options when no models available', () => {
      renderWithI18n(
        <Step6Advanced
          config={defaultConfig}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const orchestratorSelect = screen.getByLabelText(i18n.t('workflow:step6.orchestrator.label'));
      const mergeCliSelect = screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.cliLabel'));
      const mergeModelSelect = screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.modelLabel'));

      expect(orchestratorSelect).toHaveValue('');
      expect(mergeCliSelect).toHaveValue('');
      expect(mergeModelSelect).toHaveValue('');
    });
  });

  describe('CLI Type Options', () => {
    it('should display all CLI type options', () => {
      renderWithI18n(
        <Step6Advanced
          config={configWithModels}
          onUpdate={mockOnUpdate}
          errors={{}}
        />
      );

      const mergeCliSelect = screen.getByLabelText(i18n.t('workflow:step6.mergeTerminal.cliLabel'));
      fireEvent.change(mergeCliSelect, { target: { value: '' } });

      const options = screen.getAllByText(/Claude Code|Gemini CLI|Codex/);
      expect(options.length).toBeGreaterThanOrEqual(3);
    });
  });
});
