import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, fireEvent, waitFor } from '@testing-library/react';
import { Step3Models } from './Step3Models';
import type { WizardConfig } from '../types';
import { renderWithI18n, setTestLanguage, i18n } from '@/test/renderWithI18n';

const mockConfirmDialogShow = vi.fn();
vi.mock('@/components/ui-new/dialogs/ConfirmDialog', () => ({
  ConfirmDialog: {
    show: (...args: unknown[]) => mockConfirmDialogShow(...args),
  },
}));

vi.mock('@/hooks/useNativeCredentials', () => ({
  useNativeCredentials: () => ({ data: undefined, isLoading: false }),
}));

describe('Step3Models', () => {
  const mockOnUpdate = vi.fn();

  beforeEach(async () => {
    mockOnUpdate.mockClear();
    mockConfirmDialogShow.mockClear();
    await setTestLanguage();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const defaultConfig: WizardConfig = {
    project: {
      workingDirectory: '/test',
      gitStatus: { isGitRepo: true, isDirty: false },
    },
    basic: {
      name: 'Test Workflow',
      taskCount: 1,
      importFromKanban: false,
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

  it('should render empty state when no models configured', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    expect(screen.getByText(i18n.t('workflow:step3.title'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:step3.emptyTitle'))).toBeInTheDocument();
  });

  it('should render add model button', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    expect(screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') })).toBeInTheDocument();
  });

  it('should open dialog when add model button is clicked', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    expect(screen.getByRole('heading', { name: i18n.t('workflow:step3.dialog.addTitle') })).toBeInTheDocument();
    expect(screen.getByLabelText(i18n.t('workflow:step3.fields.displayName.label'))).toBeInTheDocument();
    await waitFor(() => {
      expect(warnSpy).not.toHaveBeenCalled();
    });
    warnSpy.mockRestore();

  });

  it('should render list of configured models', () => {
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
      ],
    };

    renderWithI18n(
      <Step3Models
        config={configWithModels}
        onUpdate={mockOnUpdate}
      />
    );

    expect(screen.getByText('Claude 3.5')).toBeInTheDocument();
    // The model ID should be present
    expect(screen.getByText(/claude-3-5-sonnet-20241022/)).toBeInTheDocument();
  });

  it('should display verified checkmark for verified models', () => {
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
      ],
    };

    renderWithI18n(
      <Step3Models
        config={configWithModels}
        onUpdate={mockOnUpdate}
      />
    );

    // Check for verified indicator (checkmark icon or similar)
    const verifiedBadge = screen.getByTestId(/verified-badge-model-1/i);
    expect(verifiedBadge).toBeInTheDocument();
  });

  it('should allow editing a model', () => {
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
      ],
    };

    renderWithI18n(
      <Step3Models
        config={configWithModels}
        onUpdate={mockOnUpdate}
      />
    );

    const editButton = screen.getByRole('button', { name: `${i18n.t('workflow:step3.editLabel')} Claude 3.5` });
    fireEvent.click(editButton);

    expect(screen.getByLabelText(i18n.t('workflow:step3.fields.displayName.label'))).toHaveValue('Claude 3.5');
  });

  it('should allow deleting a model', async () => {
    mockConfirmDialogShow.mockResolvedValue('confirmed');

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
      ],
    };

    renderWithI18n(
      <Step3Models
        config={configWithModels}
        onUpdate={mockOnUpdate}
      />
    );

    const deleteButton = screen.getByTitle(i18n.t('workflow:step3.deleteLabel'));
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(mockConfirmDialogShow).toHaveBeenCalledWith(
        expect.objectContaining({
          title: i18n.t('workflow:step3.messages.confirmDeleteTitle'),
          message: i18n.t('workflow:step3.messages.confirmDelete', { name: 'Claude 3.5' }),
          variant: 'destructive',
        })
      );
    });
    await waitFor(() => {
      expect(mockOnUpdate).toHaveBeenCalled();
    });
  });

  it('should auto-fill base URL based on API type selection', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    // Click on Anthropic radio button label
    const anthropicLabel = screen.getByText('Anthropic');
    fireEvent.click(anthropicLabel);

    const baseUrlInput = screen.getByLabelText(i18n.t('workflow:step3.fields.baseUrl.label'));
    expect(baseUrlInput).toHaveValue('https://api.anthropic.com');
  });

  it('should allow manual base URL input for openai-compatible', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    // Click on OpenAI Compatible radio button label
    const compatibleLabel = screen.getByText('OpenAI Compatible');
    fireEvent.click(compatibleLabel);

    const baseUrlInput = screen.getByLabelText(i18n.t('workflow:step3.fields.baseUrl.label'));
    expect(baseUrlInput).toHaveValue('');
    expect(baseUrlInput).not.toBeDisabled();
  });

  it('should show validation errors for required fields', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    const saveButton = screen.getByRole('button', { name: i18n.t('workflow:step3.actions.save') });
    fireEvent.click(saveButton);

    expect(screen.getByText(i18n.t('workflow:step3.errors.displayNameRequired'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:step3.errors.apiKeyRequired'))).toBeInTheDocument();
  });

  it('should handle API key input with password type', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    const apiKeyInput = screen.getByLabelText(i18n.t('workflow:step3.fields.apiKey.label'));
    expect(apiKeyInput).toHaveAttribute('type', 'password');
  });

  it('should render all API type options', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    expect(screen.getByText('Anthropic')).toBeInTheDocument();
    expect(screen.getByText('Google')).toBeInTheDocument();
    expect(screen.getByText('OpenAI')).toBeInTheDocument();
    expect(screen.getByText('OpenAI Compatible')).toBeInTheDocument();
  });

  it('should close dialog when cancel is clicked', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    const cancelButton = screen.getByRole('button', { name: i18n.t('workflow:step3.actions.cancel') });
    fireEvent.click(cancelButton);

    expect(screen.queryByLabelText(i18n.t('workflow:step3.fields.displayName.label'))).not.toBeInTheDocument();
  });

  it('should display multiple models', () => {
    const configWithMultipleModels: WizardConfig = {
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
          isVerified: false,
        },
      ],
    };

    renderWithI18n(
      <Step3Models
        config={configWithMultipleModels}
        onUpdate={mockOnUpdate}
      />
    );

    expect(screen.getByText('Claude 3.5')).toBeInTheDocument();
    expect(screen.getByText('GPT-4')).toBeInTheDocument();
  });

  it('should show fetch models button', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    expect(screen.getByRole('button', { name: i18n.t('workflow:step3.actions.fetchModels') })).toBeInTheDocument();
  });

  it('should show verify connection button', () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    expect(screen.getByRole('button', { name: i18n.t('workflow:step3.actions.verify') })).toBeInTheDocument();
  });

  it('should display model selection dropdown when models are fetched', async () => {
    renderWithI18n(
      <Step3Models
        config={defaultConfig}
        onUpdate={mockOnUpdate}
      />
    );

    const addButton = screen.getByRole('button', { name: i18n.t('workflow:step3.addModel') });
    fireEvent.click(addButton);

    // Fill in API Key
    fireEvent.change(screen.getByLabelText(i18n.t('workflow:step3.fields.apiKey.label')), { target: { value: 'sk-test-key' } });

    // Click fetch models
    const fetchButton = screen.getByRole('button', { name: i18n.t('workflow:step3.actions.fetchModels') });
    fireEvent.click(fetchButton);

    // After fetching, model selection should be available (dropdown with options)
    await waitFor(() => {
      const modelIdSelect = screen.getByLabelText(i18n.t('workflow:step3.fields.modelId.label'));
      expect(modelIdSelect).toBeInTheDocument();
      expect(modelIdSelect.tagName).toBe('SELECT');
    });
  });
});
