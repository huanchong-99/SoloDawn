import { renderHook, act } from '@testing-library/react';
import { useWizardNavigation } from '../useWizardNavigation';
import { WizardStep } from '../../types';

describe('useWizardNavigation', () => {
  it('starts on Project step by default', () => {
    const { result } = renderHook(() => useWizardNavigation());
    expect(result.current.currentStep).toBe(WizardStep.Project);
  });

  it('moves to next step', () => {
    const { result } = renderHook(() => useWizardNavigation());
    act(() => {
      result.current.next();
    });
    expect(result.current.currentStep).toBe(WizardStep.Basic);
  });

  it('does not move past last step', () => {
    const { result } = renderHook(() =>
      useWizardNavigation({ initialStep: WizardStep.Advanced })
    );
    act(() => {
      result.current.next();
    });
    expect(result.current.currentStep).toBe(WizardStep.Advanced);
  });

  it('moves to previous step', () => {
    const { result } = renderHook(() =>
      useWizardNavigation({ initialStep: WizardStep.Basic })
    );
    act(() => {
      result.current.previous();
    });
    expect(result.current.currentStep).toBe(WizardStep.Project);
  });

  it('does not move before first step', () => {
    const { result } = renderHook(() => useWizardNavigation());
    act(() => {
      result.current.previous();
    });
    expect(result.current.currentStep).toBe(WizardStep.Project);
  });

  it('reports when it cannot go next', () => {
    const { result } = renderHook(() =>
      useWizardNavigation({ initialStep: WizardStep.Advanced })
    );
    expect(result.current.canGoNext()).toBe(false);
  });

  it('reports when it cannot go previous', () => {
    const { result } = renderHook(() => useWizardNavigation());
    expect(result.current.canGoPrevious()).toBe(false);
  });
});
