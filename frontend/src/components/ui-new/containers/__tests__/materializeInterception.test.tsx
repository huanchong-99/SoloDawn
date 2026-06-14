import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';

import { usePlanningDraftActions } from '../CreateChatBoxContainer';

// ---------------------------------------------------------------------------
// Spec §3.7 / §6: materialize interception.
//   With gatesConfirmedAt == null, clicking Materialize opens the
//   QualityGateConfirmDialog (setGatesDialogOpen(true)) and does NOT call
//   materializeMutation. Once gates are confirmed (non-null timestamp), the
//   same action proceeds to materialize.
// We drive the REAL usePlanningDraftActions hook (the production interception
// logic in handleMaterialize), mocking only the injected mutations/setters.
// ---------------------------------------------------------------------------

function makeMutation() {
  return {
    mutateAsync: vi.fn().mockResolvedValue({ workflowId: 'wf-1' }),
    mutate: vi.fn(),
    isPending: false,
  } as never;
}

function makeArgs(overrides: { gatesConfirmedAt: string | null }) {
  const materializeMutation = makeMutation();
  const setGatesDialogOpen = vi.fn();
  const args = {
    planningDraftId: 'draft-1',
    planningDraft: { feishuSync: false },
    gatesConfirmedAt: overrides.gatesConfirmedAt,
    setGatesDialogOpen,
    message: '',
    setMessage: vi.fn(),
    setIsThinking: vi.fn(),
    setLocalMessages: vi.fn(),
    setMaterializedWorkflowId: vi.fn(),
    sendMessageMutation: makeMutation(),
    confirmMutation: makeMutation(),
    materializeMutation,
    feishuSyncMutation: makeMutation(),
    showToast: vi.fn(),
    retainBuiltin: false,
  } as never;
  return { args, materializeMutation, setGatesDialogOpen };
}

describe('materialize interception (spec §3.7 / §6)', () => {
  it('opens the dialog and does NOT call materialize when gatesConfirmedAt == null', () => {
    const { args, materializeMutation, setGatesDialogOpen } = makeArgs({
      gatesConfirmedAt: null,
    });

    const { result } = renderHook(() => usePlanningDraftActions(args));

    act(() => {
      result.current.handleMaterialize();
    });

    // Dialog opened…
    expect(setGatesDialogOpen).toHaveBeenCalledTimes(1);
    expect(setGatesDialogOpen).toHaveBeenCalledWith(true);
    // …and materialize was NOT triggered.
    expect(
      (materializeMutation as { mutateAsync: ReturnType<typeof vi.fn> })
        .mutateAsync
    ).not.toHaveBeenCalled();
  });

  it('proceeds to materialize (no dialog) when gates are already confirmed', async () => {
    const { args, materializeMutation, setGatesDialogOpen } = makeArgs({
      gatesConfirmedAt: '2026-06-14T00:00:00Z',
    });

    const { result } = renderHook(() => usePlanningDraftActions(args));

    await act(async () => {
      result.current.handleMaterialize();
      // allow the void proceedMaterialize() promise chain to settle
      await Promise.resolve();
    });

    expect(setGatesDialogOpen).not.toHaveBeenCalled();
    expect(
      (materializeMutation as { mutateAsync: ReturnType<typeof vi.fn> })
        .mutateAsync
    ).toHaveBeenCalledTimes(1);
    expect(
      (materializeMutation as { mutateAsync: ReturnType<typeof vi.fn> })
        .mutateAsync
    ).toHaveBeenCalledWith('draft-1');
  });
});
