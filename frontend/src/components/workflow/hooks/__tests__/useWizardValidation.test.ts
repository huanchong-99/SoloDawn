import { renderHook, act } from '@testing-library/react';
import { useWizardValidation } from '../useWizardValidation';
import { WizardStep, getDefaultWizardConfig } from '../../types';

describe('useWizardValidation', () => {
  it('validates the current step', () => {
    const { result } = renderHook(() =>
      useWizardValidation(WizardStep.Project)
    );

    const config = getDefaultWizardConfig();
    config.project.workingDirectory = '';

    let errors: Record<string, string> = {};
    act(() => {
      errors = result.current.validate(config);
    });
    expect(errors.workingDirectory).toBeDefined();
  });

  it('returns empty errors for valid config', () => {
    const { result } = renderHook(() =>
      useWizardValidation(WizardStep.Project)
    );

    const config = getDefaultWizardConfig();
    config.project.workingDirectory = '/valid/path';
    config.project.gitStatus = { isGitRepo: true, isDirty: false };

    let errors: Record<string, string> = {};
    act(() => {
      errors = result.current.validate(config);
    });
    expect(Object.keys(errors)).toHaveLength(0);
  });

  it('updates errors state', () => {
    const { result } = renderHook(() =>
      useWizardValidation(WizardStep.Project)
    );

    const newErrors = { workingDirectory: 'Required' };
    act(() => {
      result.current.setErrors(newErrors);
    });

    expect(result.current.errors).toEqual(newErrors);
  });

  it('clears errors', () => {
    const { result } = renderHook(() =>
      useWizardValidation(WizardStep.Project)
    );

    act(() => {
      result.current.setErrors({ field: 'error' });
    });
    act(() => {
      result.current.clearErrors();
    });

    expect(result.current.errors).toEqual({});
  });

});
