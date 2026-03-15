import React, { useState, useCallback, useMemo } from 'react';
import {
  Download,
  Trash2,
  RefreshCw,
  Loader2,
  Check,
  AlertCircle,
} from 'lucide-react';
import {
  useCliTypes,
  useCliDetection,
  type CliType,
  type CliDetectionResult,
  cliTypesKeys,
} from '@/hooks/useCliTypes';
import { useCliInstall } from '@/hooks/useCliInstall';
import { useCliStatusStream } from '@/hooks/useCliStatusStream';
import { CliInstallProgress } from './CliInstallProgress';
import { useQueryClient } from '@tanstack/react-query';

// ============================================================================
// Types
// ============================================================================

interface ActiveInstall {
  cliTypeId: string;
  jobId: string;
  action: 'install' | 'uninstall';
}

interface CliRowData {
  cliType: CliType;
  detection: CliDetectionResult | undefined;
}

// ============================================================================
// Sub-components
// ============================================================================

function StatusBadge({
  detection,
  isActive,
}: {
  readonly detection: CliDetectionResult | undefined;
  readonly isActive: boolean;
}) {
  if (isActive) {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-blue-500/20 px-2 py-0.5 text-xs text-blue-400">
        <Loader2 className="h-3 w-3 animate-spin" />
        Installing
      </span>
    );
  }

  if (!detection || !detection.isInstalled) {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-red-500/20 px-2 py-0.5 text-xs text-red-400">
        <AlertCircle className="h-3 w-3" />
        Not found
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-green-500/20 px-2 py-0.5 text-xs text-green-400">
      <Check className="h-3 w-3" />
      Installed
    </span>
  );
}

// ============================================================================
// Main component
// ============================================================================

/**
 * CLI Management Panel showing all CLIs in a table with status, version,
 * and install/uninstall actions. Supports real-time progress streaming
 * and SSE-based status updates.
 */
export function CliManagementPanel() {
  const queryClient = useQueryClient();
  const { data: cliTypes, isLoading: isLoadingTypes } = useCliTypes();
  const { data: detectionResults, isLoading: isLoadingDetection } =
    useCliDetection();
  const { installMutation, uninstallMutation } = useCliInstall();
  const [activeInstall, setActiveInstall] = useState<ActiveInstall | null>(
    null
  );

  // Enable SSE stream for real-time status updates
  useCliStatusStream(true);

  // Build row data by joining CLI types with detection results
  const rows: CliRowData[] = useMemo(() => {
    if (!cliTypes) return [];
    return cliTypes.map((cliType) => ({
      cliType,
      detection: detectionResults?.find((d) => d.cliTypeId === cliType.id),
    }));
  }, [cliTypes, detectionResults]);

  const handleInstall = useCallback(
    async (cliTypeId: string) => {
      try {
        const result = await installMutation.mutateAsync(cliTypeId);
        setActiveInstall({
          cliTypeId,
          jobId: result.job_id,
          action: 'install',
        });
      } catch {
        // Error is handled by React Query
      }
    },
    [installMutation]
  );

  const handleUninstall = useCallback(
    async (cliTypeId: string) => {
      try {
        const result = await uninstallMutation.mutateAsync(cliTypeId);
        setActiveInstall({
          cliTypeId,
          jobId: result.job_id,
          action: 'uninstall',
        });
      } catch {
        // Error is handled by React Query
      }
    },
    [uninstallMutation]
  );

  const handleRefreshDetection = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: cliTypesKeys.detection });
  }, [queryClient]);

  const handleInstallAll = useCallback(async () => {
    if (!rows.length) return;
    const uninstalledCli = rows.find(
      (row) => !row.detection?.isInstalled
    );
    if (uninstalledCli) {
      await handleInstall(uninstalledCli.cliType.id);
    }
  }, [rows, handleInstall]);

  const handleProgressComplete = useCallback(
    (success: boolean) => {
      if (success) {
        queryClient.invalidateQueries({ queryKey: cliTypesKeys.detection });
      }
      // Keep the progress display visible; user can dismiss or start another
    },
    [queryClient]
  );

  const isLoading = isLoadingTypes || isLoadingDetection;
  const isMutating =
    installMutation.isPending || uninstallMutation.isPending;
  const hasUninstalledClis = rows.some(
    (row) => !row.detection?.isInstalled
  );

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-high">CLI Management</h2>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={handleInstallAll}
            disabled={isMutating || isLoading || !hasUninstalledClis}
            className="inline-flex items-center gap-1.5 rounded border bg-secondary px-3 py-1.5 text-xs text-normal hover:text-high disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Download className="h-3.5 w-3.5" />
            Install All
          </button>
          <button
            type="button"
            onClick={handleRefreshDetection}
            disabled={isLoadingDetection}
            className="inline-flex items-center gap-1.5 rounded border bg-secondary px-3 py-1.5 text-xs text-normal hover:text-high disabled:cursor-not-allowed disabled:opacity-50"
          >
            <RefreshCw
              className={`h-3.5 w-3.5 ${isLoadingDetection ? 'animate-spin' : ''}`}
            />
            Refresh
          </button>
        </div>
      </div>

      {/* Loading state */}
      {isLoading && (
        <div className="flex items-center justify-center gap-2 py-8 text-low">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span className="text-sm">Detecting CLIs...</span>
        </div>
      )}

      {/* Table */}
      {!isLoading && (
        <div className="overflow-hidden rounded border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-secondary text-left text-xs text-low">
                <th className="px-3 py-2 font-medium">CLI Name</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium">Version</th>
                <th className="px-3 py-2 font-medium">Path</th>
                <th className="px-3 py-2 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const isActiveRow =
                  activeInstall?.cliTypeId === row.cliType.id;
                const isInstalled = row.detection?.isInstalled ?? false;

                return (
                  <tr
                    key={row.cliType.id}
                    className="border-b last:border-b-0"
                  >
                    <td className="px-3 py-2">
                      <div className="flex flex-col">
                        <span className="font-medium text-high">
                          {row.cliType.displayName}
                        </span>
                        <span className="text-xs text-low">
                          {row.cliType.name}
                        </span>
                      </div>
                    </td>
                    <td className="px-3 py-2">
                      <StatusBadge
                        detection={row.detection}
                        isActive={
                          isActiveRow && activeInstall !== null && !isMutating
                        }
                      />
                    </td>
                    <td className="px-3 py-2 text-normal">
                      {row.detection?.version ?? (
                        <span className="text-low">-</span>
                      )}
                    </td>
                    <td className="max-w-[200px] truncate px-3 py-2 font-ibm-plex-mono text-xs text-low">
                      {row.detection?.path ?? '-'}
                    </td>
                    <td className="px-3 py-2 text-right">
                      {isInstalled ? (
                        <button
                          type="button"
                          onClick={() => handleUninstall(row.cliType.id)}
                          disabled={isMutating}
                          className="inline-flex items-center gap-1 rounded px-2 py-1 text-xs text-red-400 hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                          <Trash2 className="h-3 w-3" />
                          Uninstall
                        </button>
                      ) : (
                        <button
                          type="button"
                          onClick={() => handleInstall(row.cliType.id)}
                          disabled={isMutating}
                          className="inline-flex items-center gap-1 rounded px-2 py-1 text-xs text-blue-400 hover:bg-blue-500/10 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                          <Download className="h-3 w-3" />
                          Install
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
              {rows.length === 0 && (
                <tr>
                  <td
                    colSpan={5}
                    className="px-3 py-8 text-center text-sm text-low"
                  >
                    No CLI types configured.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* Active install progress */}
      {activeInstall && (
        <div className="flex flex-col gap-2">
          <span className="text-xs font-medium text-low">
            {activeInstall.action === 'install'
              ? 'Installing'
              : 'Uninstalling'}{' '}
            CLI...
          </span>
          <CliInstallProgress
            cliTypeId={activeInstall.cliTypeId}
            jobId={activeInstall.jobId}
            onComplete={handleProgressComplete}
          />
        </div>
      )}
    </div>
  );
}
