import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, fireEvent } from '@testing-library/react';
import { Step1Basic } from './Step1Basic';
import type { BasicConfig } from '../types';
import { renderWithI18n, setTestLanguage, i18n } from '@/test/renderWithI18n';

describe('Step1Basic', () => {
  const mockOnChange = vi.fn();

  const defaultConfig: BasicConfig = {
    name: '',
    executionMode: 'diy',
    initialGoal: '',
    taskCount: 1,
    importFromKanban: false,
  };

  beforeEach(() => {
    mockOnChange.mockClear();
    void setTestLanguage();
  });

  it('should render basic configuration form', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText(i18n.t('workflow:step1.nameLabel'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:step1.taskCountLabel'))).toBeInTheDocument();
  });

  it('should always show task count and import controls', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText(i18n.t('workflow:step1.taskCountLabel'))).toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:step1.importLabel'))).toBeInTheDocument();
  });

  it('should display error when name is empty', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{ name: 'validation.basic.nameRequired' }}
      />
    );

    expect(
      screen.getByText(i18n.t('workflow:validation.basic.nameRequired'))
    ).toBeInTheDocument();
  });

  it('should allow selecting task count', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const twoTasksButton = screen.getByText(
      i18n.t('workflow:step1.taskCountOption', { count: 2 })
    );
    fireEvent.click(twoTasksButton);

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({ taskCount: 2 })
    );
  });

  it('should allow switching between import modes', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const importRadio = screen.getByText(i18n.t('workflow:step1.importKanban'));
    fireEvent.click(importRadio);

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({ importFromKanban: true })
    );
  });

  it('should render description textarea', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(
      screen.getByText(i18n.t('workflow:step1.descriptionLabel'))
    ).toBeInTheDocument();
  });

  it('should hide task count and import in agent_planned mode', () => {
    const agentConfig: BasicConfig = {
      ...defaultConfig,
      executionMode: 'agent_planned',
    };

    renderWithI18n(
      <Step1Basic
        config={agentConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.queryByText(i18n.t('workflow:step1.taskCountLabel'))).not.toBeInTheDocument();
    expect(screen.queryByText(i18n.t('workflow:step1.importLabel'))).not.toBeInTheDocument();
    expect(screen.getByText(i18n.t('workflow:step1.initialGoalLabel'))).toBeInTheDocument();
  });

  it('should render task count selection buttons', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(
      screen.getByText(i18n.t('workflow:step1.taskCountOption', { count: 1 }))
    ).toBeInTheDocument();
    expect(
      screen.getByText(i18n.t('workflow:step1.taskCountOption', { count: 2 }))
    ).toBeInTheDocument();
    expect(
      screen.getByText(i18n.t('workflow:step1.taskCountOption', { count: 3 }))
    ).toBeInTheDocument();
    expect(
      screen.getByText(i18n.t('workflow:step1.taskCountOption', { count: 4 }))
    ).toBeInTheDocument();
  });

  it('should render custom task count input', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(
      screen.getByPlaceholderText(i18n.t('workflow:step1.customCountPlaceholder'))
    ).toBeInTheDocument();
  });

  it('should handle workflow name input change', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const nameInput = screen.getByPlaceholderText(
      i18n.t('workflow:step1.namePlaceholder')
    );
    fireEvent.change(nameInput, { target: { value: 'Test Workflow' } });

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Test Workflow' })
    );
  });

  it('should handle description input change', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const descriptionInput = screen.getByPlaceholderText(
      i18n.t('workflow:step1.descriptionPlaceholder')
    );
    fireEvent.change(descriptionInput, { target: { value: 'Test description' } });

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({ description: 'Test description' })
    );
  });

  it('should display error for taskCount', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{ taskCount: 'validation.basic.taskCountMin' }}
      />
    );

    expect(
      screen.getByText(i18n.t('workflow:validation.basic.taskCountMin'))
    ).toBeInTheDocument();
  });

  it('should highlight selected task count button', () => {
    const configWithTwoTasks: BasicConfig = {
      ...defaultConfig,
      taskCount: 2,
    };

    renderWithI18n(
      <Step1Basic
        config={configWithTwoTasks}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const twoTasksButton = screen.getByText(
      i18n.t('workflow:step1.taskCountOption', { count: 2 })
    );
    expect(twoTasksButton.closest('button')).toHaveClass('border-brand');
  });

  it('should allow custom task count input', () => {
    renderWithI18n(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const customInput = screen.getByPlaceholderText(
      i18n.t('workflow:step1.customCountPlaceholder')
    );
    fireEvent.change(customInput, { target: { value: '7' } });

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({ taskCount: 7 })
    );
  });
});
