import { useMemo, useCallback, type RefObject } from 'react';
import { useActions } from '@/contexts/ActionsContext';
import { useUserSystem } from '@/components/ConfigProvider';
import { ContextBar } from '../primitives/ContextBar';
import {
  ContextBarActionGroups,
  type ActionDefinition,
} from '../actions';
import { useActionVisibilityContext } from '../actions/useActionVisibility';
import { filterVisibleItemPair } from './NavbarContainer';

export interface ContextBarContainerProps {
  readonly containerRef: RefObject<HTMLElement | null>;
}

export function ContextBarContainer({
  containerRef,
}: Readonly<ContextBarContainerProps>) {
  const { executorContext } = useActions();
  const { config } = useUserSystem();
  const editorType = config?.editor?.editorType ?? null;

  // Get visibility context (now includes dev server state)
  const actionCtx = useActionVisibilityContext();

  // Action handler - use executor context directly from provider
  const handleExecuteAction = useCallback(
    async (action: ActionDefinition) => {
      if (action.requiresTarget === false) {
        await action.execute(executorContext);
      }
    },
    [executorContext]
  );

  // Filter visible actions
  const [primaryItems, secondaryItems] = useMemo(
    () =>
      filterVisibleItemPair(
        ContextBarActionGroups.primary,
        ContextBarActionGroups.secondary,
        actionCtx
      ),
    [actionCtx]
  );

  return (
    <ContextBar
      containerRef={containerRef}
      primaryItems={primaryItems}
      secondaryItems={secondaryItems}
      actionContext={actionCtx}
      onExecuteAction={handleExecuteAction}
      editorType={editorType}
    />
  );
}
