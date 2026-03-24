import { useState, useMemo, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
import { useScratch } from '@/hooks/useScratch';
import { ScratchType, type DraftWorkspaceData } from 'shared/types';
import { splitMessageToTitleDescription } from '@/utils/string';
import {
  PERSIST_KEYS,
  usePersistedExpanded,
} from '@/stores/useUiPreferencesStore';
import { WorkspacesSidebar } from '@/components/ui-new/views/WorkspacesSidebar';
import { useConciergeSessions } from '@/hooks/useConcierge';
import { usePlanningDrafts } from '@/hooks/usePlanningDraft';
import type { Workspace } from '@/components/ui-new/hooks/useWorkspaces';

// Fixed UUID for the universal workspace draft (same as in useCreateModeState.ts)
const DRAFT_WORKSPACE_ID = '00000000-0000-0000-0000-000000000001';

export function WorkspacesSidebarContainer() {
  const {
    workspaceId: selectedWorkspaceId,
    activeWorkspaces,
    archivedWorkspaces,
    isCreateMode,
    selectWorkspace,
    navigateToCreate,
  } = useWorkspaceContext();

  const [searchQuery, setSearchQuery] = useState('');
  const [showArchive, setShowArchive] = usePersistedExpanded(
    PERSIST_KEYS.workspacesSidebarArchived,
    false
  );

  // Read persisted draft for sidebar placeholder
  const { scratch: draftScratch } = useScratch(
    ScratchType.DRAFT_WORKSPACE,
    DRAFT_WORKSPACE_ID
  );

  // Extract draft title from persisted scratch
  const persistedDraftTitle = useMemo(() => {
    const scratchData: DraftWorkspaceData | undefined =
      draftScratch?.payload?.type === 'DRAFT_WORKSPACE'
        ? draftScratch.payload.data
        : undefined;

    if (!scratchData?.message?.trim()) return undefined;
    const { title } = splitMessageToTitleDescription(
      scratchData.message.trim()
    );
    return title || 'New Workspace';
  }, [draftScratch]);

  const { data: conciergeSessions } = useConciergeSessions();
  const navigate = useNavigate();

  // Fetch planning drafts (cross-project) and merge into the workspace list
  const { data: planningDrafts } = usePlanningDrafts();
  const combinedWorkspaces = useMemo(() => {
    const draftWorkspaces: Workspace[] = (planningDrafts ?? []).map((draft) => ({
      id: `draft-${draft.id}`,
      taskId: draft.id,
      name: draft.name || 'Untitled',
      description: draft.status,
      isRunning: draft.status === 'gathering' || draft.status === 'spec_ready' || draft.status === 'confirmed',
      latestProcessStatus: draft.status === 'materialized' ? 'completed' as const : 'running' as const,
    }));
    return [...draftWorkspaces, ...activeWorkspaces];
  }, [planningDrafts, activeWorkspaces]);

  // Route draft clicks to the create page with draftId param
  const handleSelectWorkspace = useCallback((id: string) => {
    if (id.startsWith('draft-')) {
      const draftId = id.slice(6); // Remove 'draft-' prefix
      navigate(`/workspaces/create?draftId=${draftId}`);
    } else {
      selectWorkspace(id);
    }
  }, [navigate, selectWorkspace]);

  return (
    <WorkspacesSidebar
      workspaces={combinedWorkspaces}
      archivedWorkspaces={archivedWorkspaces}
      selectedWorkspaceId={selectedWorkspaceId ?? null}
      onSelectWorkspace={handleSelectWorkspace}
      searchQuery={searchQuery}
      onSearchChange={setSearchQuery}
      onAddWorkspace={navigateToCreate}
      isCreateMode={isCreateMode}
      draftTitle={persistedDraftTitle}
      onSelectCreate={navigateToCreate}
      showArchive={showArchive}
      onShowArchiveChange={setShowArchive}
      conciergeSessions={conciergeSessions ?? []}
    />
  );
}
