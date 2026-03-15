import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  useMemo,
  ReactNode,
} from 'react';
import { useApprovalMutation } from '@/hooks/useApprovalMutation';

interface ActiveApproval {
  approvalId: string;
  executionProcessId: string;
  timeoutAt: string;
  requestedAt: string;
}

interface ApprovalFeedbackContextType {
  activeApproval: ActiveApproval | null;
  enterFeedbackMode: (approval: ActiveApproval) => void;
  exitFeedbackMode: () => void;
  submitFeedback: (message: string) => Promise<void>;
  isSubmitting: boolean;
  error: string | null;
  isTimedOut: boolean;
}

const ApprovalFeedbackContext =
  createContext<ApprovalFeedbackContextType | null>(null);

export function useApprovalFeedback() {
  const context = useContext(ApprovalFeedbackContext);
  if (!context) {
    throw new Error(
      'useApprovalFeedback must be used within ApprovalFeedbackProvider'
    );
  }
  return context;
}

// Optional hook that doesn't throw - for components that may render outside provider
export function useApprovalFeedbackOptional() {
  return useContext(ApprovalFeedbackContext);
}

export function ApprovalFeedbackProvider({
  children,
}: Readonly<{
  children: ReactNode;
}>) {
  const [activeApproval, setActiveApproval] = useState<ActiveApproval | null>(
    null
  );
  const { denyAsync, isDenying, denyError, reset } = useApprovalMutation();

  const [isTimedOut, setIsTimedOut] = useState(false);
  useEffect(() => {
    if (!activeApproval) { setIsTimedOut(false); return; }
    const timeoutMs = new Date(activeApproval.timeoutAt).getTime() - Date.now();
    if (timeoutMs <= 0) { setIsTimedOut(true); return; }
    setIsTimedOut(false);
    const timer = setTimeout(() => setIsTimedOut(true), timeoutMs);
    return () => clearTimeout(timer);
  }, [activeApproval]);

  const enterFeedbackMode = useCallback(
    (approval: ActiveApproval) => {
      setActiveApproval(approval);
      reset();
    },
    [reset]
  );

  const exitFeedbackMode = useCallback(() => {
    setActiveApproval(null);
    reset();
  }, [reset]);

  const submitFeedback = useCallback(
    async (message: string) => {
      if (!activeApproval) return;

      // Check timeout before submitting
      if (new Date() > new Date(activeApproval.timeoutAt)) {
        throw new Error('Approval has timed out');
      }

      await denyAsync({
        approvalId: activeApproval.approvalId,
        executionProcessId: activeApproval.executionProcessId,
        reason: message.trim() || undefined,
      });
      setActiveApproval(null);
    },
    [activeApproval, denyAsync]
  );

  const contextValue = useMemo(
    () => ({
      activeApproval,
      enterFeedbackMode,
      exitFeedbackMode,
      submitFeedback,
      isSubmitting: isDenying,
      error: denyError?.message ?? null,
      isTimedOut,
    }),
    [activeApproval, enterFeedbackMode, exitFeedbackMode, submitFeedback, isDenying, denyError, isTimedOut]
  );

  return (
    <ApprovalFeedbackContext.Provider value={contextValue}>
      {children}
    </ApprovalFeedbackContext.Provider>
  );
}
