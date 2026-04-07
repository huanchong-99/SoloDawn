import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, fireEvent, waitFor } from '@testing-library/react';
import { WorkflowWizard } from './WorkflowWizard';
import { renderWithI18n, setTestLanguage, i18n } from '@/test/renderWithI18n';

vi.mock('@/components/ConfigProvider', () => ({
  useUserSystem: () => ({
    config: {
      workflow_model_library: [],
    },
    updateAndSaveConfig: vi.fn().mockResolvedValue({}),
  }),
}));

vi.mock('@/hooks/useProjectRepos', () => ({
  useProjectRepos: () => ({ data: undefined, isLoading: false }),
}));

vi.mock('@/hooks/useNativeCredentials', () => ({
  useNativeCredentials: () => ({ data: undefined, isLoading: false }),
}));

const fetchMock = vi.fn();

describe('WorkflowWizard', () => {
  beforeEach(() => {
    void setTestLanguage();
    fetchMock.mockReset();
    fetchMock.mockResolvedValue({
      ok: true,
      json: async () => ({
        success: true,
        data: {
          isGitRepo: true,
          isDirty: false,
          currentBranch: 'main',
        },
      }),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('should render wizard with step indicator', () => {
    renderWithI18n(
      <WorkflowWizard
        onComplete={vi.fn()}
        onCancel={vi.fn()}
      />
    );

    expect(screen.getByText(i18n.t('workflow:wizard.title'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:step0.fieldLabel'))).toBeInTheDocument();
  });

  it('should start at Project step (Step 0)', () => {
    renderWithI18n(
      <WorkflowWizard
        onComplete={vi.fn()}
        onCancel={vi.fn()}
      />
    );

    expect(screen.getByText(i18n.t('workflow:step0.fieldLabel'))).toBeInTheDocument();
  });

  it('should call onCancel when cancel button clicked', async () => {
    const onCancel = vi.fn();
    renderWithI18n(
      <WorkflowWizard
        onComplete={vi.fn()}
        onCancel={onCancel}
      />
    );

    const cancelButton = screen.getByText(i18n.t('workflow:wizard.buttons.cancel'));
    fireEvent.click(cancelButton);

    await waitFor(() => {
      expect(onCancel).toHaveBeenCalledTimes(1);
    });
  });

  it('always shows task and terminal steps in manual wizard', () => {
    renderWithI18n(
      <WorkflowWizard
        onComplete={vi.fn()}
        onCancel={vi.fn()}
      />
    );

    expect(screen.getByText(i18n.t('workflow:steps.tasks.name'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:steps.terminals.name'))).toBeInTheDocument();
  });
});
