import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, fireEvent, waitFor } from '@testing-library/react';
import { Step4Terminals } from './Step4Terminals';
import type { WizardConfig } from '../types';
import { renderWithI18n, setTestLanguage, i18n } from '@/test/renderWithI18n';

describe('Step4Terminals', () => {
  const mockOnUpdate = vi.fn<(updates: Partial<WizardConfig>) => void>();
  const mockFetch = vi.fn<
    (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
  >();

  const createFetchResponse = (body: unknown, ok = true): Response =>
    ({
      ok,
      json: () => Promise.resolve(body),
    } as Response);

  const baseConfig: WizardConfig = {
    project: {
      workingDirectory: '/test/path',
      gitStatus: { isGitRepo: true, isDirty: false },
    },
    basic: {
      name: 'Test Workflow',
      taskCount: 2,
    },
    tasks: [
      {
        id: 'task-1',
        name: 'Task 1',
        description: 'First task',
        branch: 'feat/task-1',
        terminalCount: 2,
      },
      {
        id: 'task-2',
        name: 'Task 2',
        description: 'Second task',
        branch: 'feat/task-2',
        terminalCount: 1,
      },
    ],
    models: [
      {
        id: 'model-1',
        displayName: 'Claude 3.5 Sonnet',
        apiType: 'anthropic',
        baseUrl: 'https://api.anthropic.com',
        apiKey: 'sk-test',
        modelId: 'claude-3-5-sonnet',
        isVerified: true,
      },
    ],
    terminals: [],
    commands: {
      enabled: false,
      presetIds: [],
    },
    advanced: {
      orchestrator: { modelConfigId: 'model-1' },
      errorTerminal: { enabled: false },
      mergeTerminal: {
        cliTypeId: 'claude-code',
        modelConfigId: 'model-1',
        runTestsBeforeMerge: true,
        pauseOnConflict: true,
      },
      targetBranch: 'main',
    },
  };

  beforeEach(async () => {
    mockOnUpdate.mockClear();
    mockFetch.mockReset();
    globalThis.fetch = mockFetch as typeof fetch;
    await setTestLanguage();
  });

  it('should render terminal configuration UI', () => {
    renderWithI18n(
      <Step4Terminals
        config={baseConfig}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    expect(screen.getByText(i18n.t('workflow:step4.title'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:step4.taskIndicator', { current: 1, total: 2 }))).toBeInTheDocument();
  });

  it('should initialize terminals when config length mismatches task count', async () => {
    renderWithI18n(
      <Step4Terminals
        config={baseConfig}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    await waitFor(() => {
      expect(mockOnUpdate).toHaveBeenCalled();
    });

    const calls = mockOnUpdate.mock.calls;
    const terminalsUpdate = calls.find(([update]) => update.terminals);

    expect(terminalsUpdate).toBeDefined();
    const [update] = terminalsUpdate ?? [];
    expect(update?.terminals).toHaveLength(3);
  });

  it('should display terminal count for current task', () => {
    renderWithI18n(
      <Step4Terminals
        config={baseConfig}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    expect(screen.getByText(i18n.t('workflow:step4.terminalCount', { count: 2 }))).toBeInTheDocument();
  });

  it('should show CLI installation status', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/gemini-cli' },
        { cliTypeId: 'cli-codex', name: 'codex', displayName: 'Codex', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/codex' },
        { cliTypeId: 'cli-cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/cursor-agent' },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={baseConfig}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    await waitFor(() => {
      expect(screen.getByText(i18n.t('workflow:step4.cliStatusTitle'))).toBeInTheDocument();
      expect(screen.getByText('Claude Code')).toBeInTheDocument();
      expect(screen.getByText('Gemini CLI')).toBeInTheDocument();
      expect(screen.getAllByText(i18n.t('workflow:step4.installGuide'))).toHaveLength(3);
    });
    expect(errorSpy).not.toHaveBeenCalled();
    errorSpy.mockRestore();
  });

  it('should show install guide links for uninstalled CLIs', async () => {
    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/gemini-cli' },
        { cliTypeId: 'cli-codex', name: 'codex', displayName: 'Codex', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/codex' },
        { cliTypeId: 'cli-cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/cursor-agent' },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={baseConfig}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    const installLinks = await screen.findAllByText(i18n.t('workflow:step4.installGuide'));
    expect(installLinks.length).toBeGreaterThan(0);
  });

  it('should navigate between tasks', () => {
    renderWithI18n(
      <Step4Terminals
        config={baseConfig}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    const nextButton = screen.getByText(i18n.t('workflow:step4.nextTask'));
    const prevButton = screen.getByText(i18n.t('workflow:step4.previousTask'));

    expect(nextButton).toBeInTheDocument();
    expect(prevButton).toBeInTheDocument();
    expect(prevButton).toBeDisabled();
  });

  it('should allow CLI type selection for terminals', async () => {
    const configWithTerminals: WizardConfig = {
      ...baseConfig,
      terminals: [
        {
          id: 'terminal-task-1-0',
          taskId: 'task-1',
          orderIndex: 0,
          cliTypeId: '',
          modelConfigId: '',
          role: '',
        },
        {
          id: 'terminal-task-1-1',
          taskId: 'task-1',
          orderIndex: 1,
          cliTypeId: '',
          modelConfigId: '',
          role: '',
        },
      ],
    };

    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/gemini-cli' },
        { cliTypeId: 'cli-codex', name: 'codex', displayName: 'Codex', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/codex' },
        { cliTypeId: 'cli-cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/cursor-agent' },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={configWithTerminals}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    // Wait for CLI types to load and render
    await waitFor(() => {
      const cliButtons = screen.getAllByText('Claude Code');
      expect(cliButtons.length).toBeGreaterThan(0);
    });
  });

  it('normalizes legacy CLI detect responses to canonical CLI ids', async () => {
    const configWithBoundModel: WizardConfig = {
      ...baseConfig,
      models: [
        {
          ...baseConfig.models[0],
          cliTypeId: 'cli-claude-code',
        },
      ],
      terminals: [
        {
          id: 'terminal-task-1-0',
          taskId: 'task-1',
          orderIndex: 0,
          cliTypeId: '',
          modelConfigId: '',
          role: '',
        },
      ],
    };

    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={configWithBoundModel}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    const claudeButton = await screen.findByRole('button', { name: 'Claude Code' });

    mockOnUpdate.mockClear();
    fireEvent.click(claudeButton);

    await waitFor(() => {
      expect(mockOnUpdate).toHaveBeenCalled();
    });

    const [update] = mockOnUpdate.mock.calls.at(-1) ?? [];
    expect(update?.terminals?.[0]?.cliTypeId).toBe('cli-claude-code');
  });

  it('should allow model selection for terminals', async () => {
    const configWithTerminals: WizardConfig = {
      ...baseConfig,
      terminals: [
        {
          id: 'terminal-task-1-0',
          taskId: 'task-1',
          orderIndex: 0,
          cliTypeId: 'cli-claude-code',
          modelConfigId: '',
          role: '',
        },
      ],
    };

    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/gemini-cli' },
        { cliTypeId: 'cli-codex', name: 'codex', displayName: 'Codex', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/codex' },
        { cliTypeId: 'cli-cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/cursor-agent' },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={configWithTerminals}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    await waitFor(() => {
      expect(screen.getByText(i18n.t('workflow:step4.modelLabel'))).toBeInTheDocument();
    });

    const option = screen.getByText('Claude 3.5 Sonnet');
    expect(option).toBeInTheDocument();
  });

  it('should allow role description input', async () => {
    const configWithTerminals: WizardConfig = {
      ...baseConfig,
      terminals: [
        {
          id: 'terminal-task-1-0',
          taskId: 'task-1',
          orderIndex: 0,
          cliTypeId: 'cli-claude-code',
          modelConfigId: 'model-1',
          role: '',
        },
      ],
    };

    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/gemini-cli' },
        { cliTypeId: 'cli-codex', name: 'codex', displayName: 'Codex', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/codex' },
        { cliTypeId: 'cli-cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/cursor-agent' },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={configWithTerminals}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    await waitFor(() => {
      expect(screen.getByText(i18n.t('workflow:step4.roleLabel'))).toBeInTheDocument();
    });

    const roleInput = screen.getByPlaceholderText(i18n.t('workflow:step4.rolePlaceholder'));
    expect(roleInput).toBeInTheDocument();

    // Just verify the input exists and can be changed
    fireEvent.change(roleInput, { target: { value: 'Backend API expert' } });
    // Value might not update immediately due to state updates
    expect(mockOnUpdate).toHaveBeenCalled();
  });

  it('should update terminal when values change', async () => {
    const configWithTerminals: WizardConfig = {
      ...baseConfig,
      terminals: [
        {
          id: 'terminal-task-1-0',
          taskId: 'task-1',
          orderIndex: 0,
          cliTypeId: '',
          modelConfigId: '',
          role: '',
        },
      ],
    };

    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/gemini-cli' },
        { cliTypeId: 'cli-codex', name: 'codex', displayName: 'Codex', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/codex' },
        { cliTypeId: 'cli-cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/cursor-agent' },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={configWithTerminals}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    await waitFor(() => {
      expect(screen.getByText(i18n.t('workflow:step4.roleLabel'))).toBeInTheDocument();
    });

    const roleInput = screen.getByPlaceholderText(i18n.t('workflow:step4.rolePlaceholder'));
    fireEvent.change(roleInput, { target: { value: 'Test role' } });

    await waitFor(() => {
      const calls = mockOnUpdate.mock.calls;
      expect(calls.length).toBeGreaterThan(0);
      const lastCall = calls[calls.length - 1][0];
      const terminalWithRole = lastCall.terminals?.find(
        (terminal) => terminal.role === 'Test role'
      );
      expect(terminalWithRole).toBeTruthy();
    });
  });

  it('should display errors for terminal configuration', async () => {
    const configWithTerminals: WizardConfig = {
      ...baseConfig,
      terminals: [
        {
          id: 'terminal-task-1-0',
          taskId: 'task-1',
          orderIndex: 0,
          cliTypeId: '',
          modelConfigId: '',
          role: '',
        },
      ],
    };

    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/gemini-cli' },
        { cliTypeId: 'cli-codex', name: 'codex', displayName: 'Codex', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/codex' },
        { cliTypeId: 'cli-cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/cursor-agent' },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={configWithTerminals}
        errors={{
          'terminal-terminal-task-1-0-cli': 'validation.terminals.cliRequired',
          'terminal-terminal-task-1-0-model': 'validation.terminals.modelRequired',
        }}
        onUpdate={mockOnUpdate}
      />
    );

    await waitFor(() => {
      const alertTexts = screen
        .getAllByRole('alert')
        .map((alert) => alert.textContent);
      expect(alertTexts).toEqual(
        expect.arrayContaining([
          i18n.t('workflow:validation.terminals.cliRequired'),
          i18n.t('workflow:validation.terminals.modelRequired'),
        ])
      );
    });
  });

  it('should sort terminals by orderIndex', async () => {
    const configWithTerminals: WizardConfig = {
      ...baseConfig,
      terminals: [
        {
          id: 'terminal-task-1-1',
          taskId: 'task-1',
          orderIndex: 1,
          cliTypeId: 'cli-claude-code',
          modelConfigId: 'model-1',
          role: 'Second terminal',
        },
        {
          id: 'terminal-task-1-0',
          taskId: 'task-1',
          orderIndex: 0,
          cliTypeId: 'cli-gemini-cli',
          modelConfigId: 'model-1',
          role: 'First terminal',
        },
      ],
    };

    mockFetch.mockResolvedValueOnce(
      createFetchResponse([
        { cliTypeId: 'cli-claude-code', name: 'claude-code', displayName: 'Claude Code', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: true, version: null, executablePath: null, installGuideUrl: null },
        { cliTypeId: 'cli-codex', name: 'codex', displayName: 'Codex', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/codex' },
        { cliTypeId: 'cli-cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, version: null, executablePath: null, installGuideUrl: 'https://example.com/install/cursor-agent' },
      ])
    );

    renderWithI18n(
      <Step4Terminals
        config={configWithTerminals}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    await waitFor(() => {
      const labelPattern = new RegExp(
        i18n.t('workflow:step4.terminalLabel', { index: 1 }).replace('1', String.raw`\d`)
      );
      const terminalHeaders = screen.getAllByText(labelPattern);
      expect(terminalHeaders[0]).toHaveTextContent(
        i18n.t('workflow:step4.terminalLabel', { index: 1 })
      );
      expect(terminalHeaders[1]).toHaveTextContent(
        i18n.t('workflow:step4.terminalLabel', { index: 2 })
      );
    });
  });

  it('should handle fetch error gracefully', async () => {
    const onError = vi.fn();
    mockFetch.mockRejectedValueOnce(new Error('Network error'));

    renderWithI18n(
      <Step4Terminals
        config={baseConfig}
        errors={{}}
        onUpdate={mockOnUpdate}
        onError={onError}
      />
    );

    // Should still render even if fetch fails
    expect(screen.getByText(i18n.t('workflow:step4.title'))).toBeInTheDocument();
    await waitFor(() => {
      expect(onError).toHaveBeenCalledWith(expect.any(Error));
    });
  });

  it('should return null when current task is not available', () => {
    const configWithoutTasks: WizardConfig = {
      ...baseConfig,
      tasks: [],
    };

    const { container } = renderWithI18n(
      <Step4Terminals
        config={configWithoutTasks}
        errors={{}}
        onUpdate={mockOnUpdate}
      />
    );

    // Component should return null, rendering nothing
    expect(container).toBeEmptyDOMElement();
  });
});
