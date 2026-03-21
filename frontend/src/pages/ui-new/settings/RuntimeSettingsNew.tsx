import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  CheckCircle,
  Warning,
  ArrowsClockwise,
  Package,
  Wrench,
  XCircle,
} from '@phosphor-icons/react';
import { handleApiResponse } from '@/lib/api';
import { cliTypesKeys } from '@/hooks/useCliTypes';

// ============================================================================
// Types
// ============================================================================

interface CliRuntimeInfo {
  name: string;
  displayName: string;
  installed: boolean;
  version?: string | null;
  installGuideUrl?: string;
}

interface PrerequisiteStatus {
  name: string;
  found: boolean;
  version?: string | null;
  required: boolean;
  hint: string;
}

interface SystemPrerequisites {
  items: PrerequisiteStatus[];
}

// ============================================================================
// Data
// ============================================================================

function useRuntimeCliStatus() {
  return useQuery<CliRuntimeInfo[]>({
    queryKey: ['runtime', 'cli-status'],
    queryFn: async () => {
      const response = await fetch('/api/cli_types/detect');
      if (!response.ok) throw new Error(`CLI detect failed: ${response.status}`);
      // This endpoint returns a raw array, not wrapped in ApiResponse
      const data: Record<string, unknown>[] = await response.json();
      return (data || []).map((item) => ({
        name: (item.name as string) || '',
        displayName: (item.displayName as string) || (item.name as string) || '',
        installed: Boolean(item.installed),
        version: item.version as string | undefined,
        installGuideUrl: item.installGuideUrl as string | undefined,
      }));
    },
    staleTime: 60_000,
  });
}

function useSystemPrerequisites() {
  return useQuery<PrerequisiteStatus[]>({
    queryKey: ['runtime', 'prerequisites'],
    queryFn: async () => {
      const response = await fetch('/api/system/prerequisites');
      const data = await handleApiResponse<SystemPrerequisites>(response);
      return data?.items || [];
    },
    staleTime: 120_000,
  });
}

// ============================================================================
// Component
// ============================================================================

export function RuntimeSettingsNew() {
  const { t } = useTranslation(['settings']);
  const queryClient = useQueryClient();
  const { data: cliStatuses, isLoading } = useRuntimeCliStatus();
  const { data: prerequisites, isLoading: prereqLoading } =
    useSystemPrerequisites();

  const refreshMutation = useMutation({
    mutationFn: async () => {
      const response = await fetch('/api/cli_types/detect', { method: 'GET' });
      return handleApiResponse(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['runtime'] });
      queryClient.invalidateQueries({ queryKey: cliTypesKeys.detection });
    },
  });

  const refreshPrereqMutation = useMutation({
    mutationFn: async () => {
      const response = await fetch('/api/system/prerequisites');
      return handleApiResponse(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['runtime', 'prerequisites'] });
    },
  });

  const missingRequired = prerequisites?.filter(
    (p) => p.required && !p.found
  );
  const allRequiredMet = missingRequired?.length === 0;
  const installedCount = cliStatuses?.filter((c) => c.installed).length ?? 0;

  return (
    <div className="space-y-double">
      {/* Header */}
      <div>
        <h2 className="text-lg font-semibold text-high">
          {t('settings:runtime.title')}
        </h2>
        <p className="mt-half text-sm text-normal">
          {t('settings:runtime.description')}
        </p>
      </div>

      {/* System Prerequisites Section */}
      <div className="rounded border p-base">
        <div className="mb-base flex items-center justify-between">
          <div className="flex items-center gap-half">
            <Wrench className="size-icon-md text-brand" weight="duotone" />
            <h3 className="text-base font-medium text-high">
              {t('settings:runtime.prerequisites')}
            </h3>
            {!prereqLoading && prerequisites && (
              <span
                className={`ml-half rounded px-half text-xs ${
                  allRequiredMet
                    ? 'bg-success/10 text-success'
                    : 'bg-error/10 text-error'
                }`}
              >
                {allRequiredMet
                  ? t('settings:runtime.prereqAllMet')
                  : t('settings:runtime.prereqMissing', {
                      count: missingRequired?.length ?? 0,
                    })}
              </span>
            )}
          </div>
          <button
            type="button"
            onClick={() => refreshPrereqMutation.mutate()}
            disabled={refreshPrereqMutation.isPending}
            className="flex items-center gap-half rounded border px-half py-1 text-xs text-low hover:text-normal disabled:opacity-50"
          >
            <ArrowsClockwise
              className={`size-icon-xs ${refreshPrereqMutation.isPending ? 'animate-spin' : ''}`}
            />
            {t('settings:runtime.refresh')}
          </button>
        </div>

        {prereqLoading ? (
          <div className="py-base text-center text-sm text-low">
            {t('settings:runtime.prereqLoading')}
          </div>
        ) : (
          <div className="space-y-half">
            {prerequisites?.map((dep) => (
              <div
                key={dep.name}
                className="flex items-center justify-between rounded bg-panel px-base py-half"
              >
                <div className="flex items-center gap-half">
                  {dep.found && (
                    <CheckCircle
                      className="size-icon-sm text-success"
                      weight="fill"
                    />
                  )}
                  {!dep.found && dep.required && (
                    <XCircle
                      className="size-icon-sm text-error"
                      weight="fill"
                    />
                  )}
                  {!dep.found && !dep.required && (
                    <Warning className="size-icon-sm text-low" />
                  )}
                  <span className="text-sm text-high">{dep.name}</span>
                  {!dep.required && (
                    <span className="text-xs text-low">
                      ({t('settings:runtime.optional')})
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-base">
                  {dep.found && dep.version && (
                    <span className="font-ibm-plex-mono text-xs text-low">
                      {dep.version}
                    </span>
                  )}
                  {!dep.found && (
                    <span className="text-xs text-low">{dep.hint}</span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* CLI Tools Section */}
      <div className="rounded border p-base">
        <div className="mb-base flex items-center justify-between">
          <div className="flex items-center gap-half">
            <Package className="size-icon-md text-brand" weight="duotone" />
            <h3 className="text-base font-medium text-high">
              {t('settings:runtime.cliTools')}
            </h3>
            {!isLoading && cliStatuses && (
              <span className="ml-half text-xs text-low">
                {t('settings:runtime.cliCount', {
                  installed: installedCount,
                  total: cliStatuses.length,
                })}
              </span>
            )}
          </div>
          <button
            type="button"
            onClick={() => refreshMutation.mutate()}
            disabled={refreshMutation.isPending}
            className="flex items-center gap-half rounded border px-half py-1 text-xs text-low hover:text-normal disabled:opacity-50"
          >
            <ArrowsClockwise
              className={`size-icon-xs ${refreshMutation.isPending ? 'animate-spin' : ''}`}
            />
            {t('settings:runtime.refresh')}
          </button>
        </div>

        {isLoading && (
          <div className="py-base text-center text-sm text-low">
            {t('settings:runtime.loading')}
          </div>
        )}
        {!isLoading && (!cliStatuses || cliStatuses.length === 0) && (
          <div className="py-base text-center text-sm text-low">
            {t('settings:runtime.noClis')}
          </div>
        )}
        {!isLoading && cliStatuses && cliStatuses.length > 0 && (
          <div className="space-y-half">
            {cliStatuses.map((cli) => (
              <div
                key={cli.name}
                className="flex items-center justify-between rounded bg-panel px-base py-half"
              >
                <div className="flex items-center gap-half">
                  {cli.installed ? (
                    <CheckCircle className="size-icon-sm text-success" weight="fill" />
                  ) : (
                    <XCircle className="size-icon-sm text-low" />
                  )}
                  <span className="text-sm text-high">{cli.displayName}</span>
                </div>
                <div className="flex items-center gap-base">
                  {cli.installed && cli.version && (
                    <span className="font-ibm-plex-mono text-xs text-low">
                      {cli.version}
                    </span>
                  )}
                  {!cli.installed && (
                    <span className="text-xs text-low">
                      {t('settings:runtime.notInstalled')}
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
