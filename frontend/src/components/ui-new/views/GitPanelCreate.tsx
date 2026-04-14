import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { CollapsibleSectionHeader } from '@/components/ui-new/containers/CollapsibleSectionHeader';
import { SelectedReposList } from '@/components/ui-new/primitives/SelectedReposList';
import { ProjectSelectorContainer } from '@/components/ui-new/containers/ProjectSelectorContainer';
import { RecentReposListContainer } from '@/components/ui-new/containers/RecentReposListContainer';
import { BrowseRepoButtonContainer } from '@/components/ui-new/containers/BrowseRepoButtonContainer';
import { CreateRepoButtonContainer } from '@/components/ui-new/containers/CreateRepoButtonContainer';
import { WarningIcon, LinkSimpleIcon } from '@phosphor-icons/react';
import { PERSIST_KEYS } from '@/stores/useUiPreferencesStore';
import type { Project, GitBranch, Repo } from 'shared/types';

interface GitPanelCreateProps {
  className?: string;
  repos: Repo[];
  projects: Project[];
  selectedProjectId: string | null;
  selectedProjectName?: string;
  onProjectSelect: (project: Project) => void;
  onCreateProject: () => void;
  onRepoRemove: (repoId: string) => void;
  branchesByRepo: Record<string, GitBranch[]>;
  targetBranches: Record<string, string>;
  onBranchChange: (repoId: string, branch: string) => void;
  registeredRepoPaths: string[];
  onRepoRegistered: (repo: Repo) => void;
  boundRepoPath: string | null;
  isBinding: boolean;
  onBindRepo: () => void;
}

export function GitPanelCreate({
  className,
  repos,
  projects,
  selectedProjectId,
  selectedProjectName,
  onProjectSelect,
  onCreateProject,
  onRepoRemove,
  branchesByRepo,
  targetBranches,
  onBranchChange,
  registeredRepoPaths,
  onRepoRegistered,
  boundRepoPath,
  isBinding,
  onBindRepo,
}: Readonly<GitPanelCreateProps>) {
  const { t } = useTranslation(['tasks', 'common']);
  const hasNoRepos = repos.length === 0;
  const firstRepoPath = repos[0]?.path ?? null;
  const isBound = boundRepoPath !== null && boundRepoPath === firstRepoPath;
  const canBind = repos.length > 0 && selectedProjectId !== null;

  return (
    <div
      className={cn(
        'w-full h-full bg-secondary flex flex-col text-low overflow-y-auto',
        className
      )}
    >
      <CollapsibleSectionHeader
        title={t('common:sections.project')}
        persistKey={PERSIST_KEYS.gitPanelProject}
        contentClassName="p-base border-b"
      >
        <ProjectSelectorContainer
          projects={projects}
          selectedProjectId={selectedProjectId}
          selectedProjectName={selectedProjectName}
          onProjectSelect={onProjectSelect}
          onCreateProject={onCreateProject}
        />
      </CollapsibleSectionHeader>

      <CollapsibleSectionHeader
        title={t('common:sections.repositories')}
        persistKey={PERSIST_KEYS.gitPanelRepositories}
        contentClassName="p-base border-b"
      >
        {hasNoRepos ? (
          <div className="flex items-center gap-2 p-base rounded bg-warning/10 border border-warning/20">
            <WarningIcon className="h-4 w-4 text-warning shrink-0" />
            <p className="text-sm text-warning">
              {t('gitPanel.create.warnings.noReposSelected')}
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-half">
            <SelectedReposList
              repos={repos}
              onRemove={onRepoRemove}
              branchesByRepo={branchesByRepo}
              selectedBranches={targetBranches}
              onBranchChange={onBranchChange}
            />
            {isBound && (
              <div className="flex items-center gap-1.5 text-xs text-success px-1">
                <LinkSimpleIcon className="h-3 w-3 shrink-0" />
                <span>
                  {t('common:workspace.repoBoundTo', {
                    project: selectedProjectName ?? '',
                  })}
                </span>
              </div>
            )}
            {canBind && selectedProjectName && (
              <button
                type="button"
                onClick={onBindRepo}
                disabled={isBinding}
                className={cn(
                  'flex items-center justify-center gap-1.5 w-full px-base py-half',
                  'text-xs rounded border bg-brand/10 border-brand/30 text-brand',
                  'hover:bg-brand/20 transition-colors',
                  'disabled:opacity-50 disabled:cursor-not-allowed'
                )}
              >
                <LinkSimpleIcon className="h-3 w-3 shrink-0" />
                {isBinding
                  ? t('common:states.saving')
                  : t('common:workspace.bindRepoTo', {
                      project: selectedProjectName ?? '',
                    })}
              </button>
            )}
          </div>
        )}
      </CollapsibleSectionHeader>
      <CollapsibleSectionHeader
        title={t('common:sections.addRepositories')}
        persistKey={PERSIST_KEYS.gitPanelAddRepositories}
        contentClassName="flex flex-col p-base gap-half"
      >
        <p className="text-xs text-low font-medium">
          {t('common:sections.recent')}
        </p>
        <RecentReposListContainer
          registeredRepoPaths={registeredRepoPaths}
          onRepoRegistered={onRepoRegistered}
        />
        <p className="text-xs text-low font-medium">
          {t('common:sections.other')}
        </p>
        <BrowseRepoButtonContainer onRepoRegistered={onRepoRegistered} />
        <CreateRepoButtonContainer onRepoCreated={onRepoRegistered} />
      </CollapsibleSectionHeader>
    </div>
  );
}
