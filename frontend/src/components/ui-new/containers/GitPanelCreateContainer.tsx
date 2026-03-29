import { useMemo, useEffect, useCallback, useState, useRef } from 'react';
import { useToast } from '@/components/ui/toast';
import { GitPanelCreate } from '@/components/ui-new/views/GitPanelCreate';
import { useMultiRepoBranches } from '@/hooks/useRepoBranches';
import { useProjects } from '@/hooks/useProjects';
import { useProjectMutations } from '@/hooks/useProjectMutations';
import { useCreateMode } from '@/contexts/CreateModeContext';
import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
import { repoApi } from '@/lib/api';

interface GitPanelCreateContainerProps {
  readonly className?: string;
}

export function GitPanelCreateContainer({
  className,
}: Readonly<GitPanelCreateContainerProps>) {
  const {
    repos,
    addRepo,
    removeRepo,
    clearRepos,
    targetBranches,
    setTargetBranch,
    selectedProjectId,
    setSelectedProjectId,
  } = useCreateMode();
  const { projects } = useProjects();
  const { updateProject } = useProjectMutations();
  const { showToast } = useToast();
  const [isBinding, setIsBinding] = useState(false);
  const autoLoadedForProject = useRef<string | null>(null);

  const repoIds = useMemo(() => repos.map((r) => r.id), [repos]);
  const { branchesByRepo } = useMultiRepoBranches(repoIds);

  // Auto-select current branch when branches load
  useEffect(() => {
    repos.forEach((repo) => {
      const branches = branchesByRepo[repo.id];
      if (branches && !targetBranches[repo.id]) {
        const currentBranch = branches.find((b) => b.is_current);
        if (currentBranch) {
          setTargetBranch(repo.id, currentBranch.name);
        }
      }
    });
  }, [repos, branchesByRepo, targetBranches, setTargetBranch]);

  const selectedProject = projects.find((p) => p.id === selectedProjectId);
  const boundRepoPath = selectedProject?.defaultAgentWorkingDir ?? null;

  // Auto-load bound repo when switching projects
  useEffect(() => {
    if (!selectedProjectId) return;
    if (autoLoadedForProject.current === selectedProjectId) return;

    autoLoadedForProject.current = selectedProjectId;

    if (!boundRepoPath) {
      clearRepos();
      return;
    }

    const loadBoundRepo = async () => {
      try {
        const repo = await repoApi.register({ path: boundRepoPath });
        clearRepos();
        addRepo(repo);
      } catch (e) {
        console.error('[GitPanelCreate] Failed to auto-load bound repo:', e);
        const err = e as { message?: string };
        showToast(err.message ?? 'Failed to load bound repository', 'error');
        clearRepos();
      }
    };
    loadBoundRepo();
  }, [selectedProjectId, boundRepoPath, clearRepos, addRepo, showToast]);

  const registeredRepoPaths = useMemo(() => repos.map((r) => r.path), [repos]);

  const handleCreateProject = useCallback(async () => {
    const result = await CreateProjectDialog.show({});
    if (result.status === 'saved') {
      setSelectedProjectId(result.project.id);
      clearRepos();
      autoLoadedForProject.current = result.project.id;
    }
  }, [setSelectedProjectId, clearRepos]);

  const handleBindRepo = useCallback(async () => {
    if (!selectedProjectId || repos.length === 0) return;
    const repoPath = repos[0].path;
    setIsBinding(true);
    try {
      await updateProject.mutateAsync({
        projectId: selectedProjectId,
        data: { name: null, defaultAgentWorkingDir: repoPath },
      });
    } catch (e) {
      console.error('[GitPanelCreate] Failed to bind repo:', e);
      const err = e as { message?: string };
      showToast(err.message ?? 'Failed to bind repository', 'error');
    } finally {
      setIsBinding(false);
    }
  }, [selectedProjectId, repos, updateProject, showToast]);

  return (
    <GitPanelCreate
      className={className}
      repos={repos}
      projects={projects}
      selectedProjectId={selectedProjectId}
      selectedProjectName={selectedProject?.name}
      onProjectSelect={(p) => {
        setSelectedProjectId(p.id);
        autoLoadedForProject.current = null;
      }}
      onCreateProject={handleCreateProject}
      onRepoRemove={removeRepo}
      branchesByRepo={branchesByRepo}
      targetBranches={targetBranches}
      onBranchChange={setTargetBranch}
      registeredRepoPaths={registeredRepoPaths}
      onRepoRegistered={addRepo}
      boundRepoPath={boundRepoPath}
      isBinding={isBinding}
      onBindRepo={handleBindRepo}
    />
  );
}
