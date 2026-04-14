import { useTerminalLatestQuality, useQualityIssues } from '@/hooks/useQualityGate';
import { isQualityGateAvailable } from '@/lib/apiVersionCompat';
import { QualityBadge, type GateStatus } from '@/components/workflow/QualityBadge';
import { QualityIssueList } from './QualityIssueList';
import { AlertTriangle, StopCircle, Bug, Loader2, ShieldOff } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useTranslation } from 'react-i18next';

export interface QualityReportPanelProps {
  terminalId: string;
  className?: string;
  onRefresh?: () => void;
}

export function QualityReportPanel({ terminalId, className, onRefresh }: Readonly<QualityReportPanelProps>) {
  const { t } = useTranslation('quality');
  const { data: latestRun, isLoading, error, refetch } = useTerminalLatestQuality(terminalId);
  const runId = latestRun?.id;
  const { data: issuesData, isLoading: issuesLoading } = useQualityIssues(runId);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-8 text-slate-500">
        <Loader2 className="w-6 h-6 animate-spin mr-2" />
        <span className="text-sm">{t('panel.loading')}</span>
      </div>
    );
  }

  // Fallback when backend doesn't support quality gate (404 / version mismatch)
  if (error && !isQualityGateAvailable(error)) {
    return (
      <div className="flex flex-col items-center justify-center p-8 text-slate-400 text-sm border border-dashed rounded-md">
        <ShieldOff className="w-8 h-8 mb-2 text-slate-300" />
        <span>{t('panel.notAvailable', 'Quality gate not available')}</span>
        <span className="text-xs mt-1 text-slate-300">
          {t('panel.notAvailableHint', 'The backend does not support this feature yet.')}
        </span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-4 bg-red-50 text-red-600 rounded-md text-sm border border-red-100">
        {t('panel.error')}
      </div>
    );
  }

  if (!isLoading && !latestRun) {
    return (
      <div className="text-center p-8 text-slate-500 text-sm border border-dashed rounded-md">
        {t('panel.empty')}
      </div>
    );
  }

  const issues = issuesData || [];
  const metrics = {
    total: latestRun?.totalIssues ?? 0,
    blocker: 0,
    critical: 0,
    major: 0,
    minor: 0,
    info: 0,
  };
  // Derive metrics from issues if available
  for (const issue of issues) {
    const sev = issue.severity?.toLowerCase();
    if (sev === 'blocker') metrics.blocker++;
    else if (sev === 'critical') metrics.critical++;
    else if (sev === 'major') metrics.major++;
    else if (sev === 'minor') metrics.minor++;
    else metrics.info++;
  }

  return (
    <div className={className}>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-100">
            {t('panel.title')}
          </h3>
          <div className="text-xs text-slate-500 mt-1">
            {t('panel.runId')}: <span className="font-mono">{latestRun?.id.substring(0, 8)}</span>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm" onClick={async () => { await refetch(); onRefresh?.(); }}>
            {t('panel.refresh')}
          </Button>
          <QualityBadge gateStatus={(latestRun?.gateStatus ?? 'pending') as GateStatus} />
        </div>
      </div>

      <div className="grid grid-cols-4 gap-4 mb-6">
        <div className="bg-slate-50 dark:bg-slate-900 p-4 rounded-lg border border-slate-100 dark:border-slate-800 flex flex-col items-center justify-center">
          <div className="text-2xl font-bold text-slate-900 dark:text-slate-100">{metrics?.total || 0}</div>
          <div className="text-xs text-slate-500 uppercase tracking-wider font-semibold mt-1">{t('metrics.totalIssues')}</div>
        </div>
        
        <div className="bg-red-50 dark:bg-red-950/20 p-4 rounded-lg border border-red-100 dark:border-red-900/30 flex flex-col items-center justify-center">
          <StopCircle className="w-5 h-5 text-red-500 mb-1" />
          <div className="text-xl font-bold text-red-700 dark:text-red-400">{metrics?.blocker || 0}</div>
          <div className="text-[10px] text-red-600 dark:text-red-500 uppercase tracking-wider font-semibold mt-0.5">{t('metrics.blockers')}</div>
        </div>

        <div className="bg-amber-50 dark:bg-amber-950/20 p-4 rounded-lg border border-amber-100 dark:border-amber-900/30 flex flex-col items-center justify-center">
          <AlertTriangle className="w-5 h-5 text-amber-500 mb-1" />
          <div className="text-xl font-bold text-amber-700 dark:text-amber-400">{(metrics?.critical ?? 0) + (metrics?.major ?? 0)}</div>
          <div className="text-[10px] text-amber-600 dark:text-amber-500 uppercase tracking-wider font-semibold mt-0.5">{t('metrics.criticalMajor')}</div>
        </div>

        <div className="bg-yellow-50 dark:bg-yellow-950/20 p-4 rounded-lg border border-yellow-100 dark:border-yellow-900/30 flex flex-col items-center justify-center">
          <Bug className="w-5 h-5 text-yellow-500 mb-1" />
          <div className="text-xl font-bold text-yellow-700 dark:text-yellow-400">{(metrics?.minor ?? 0) + (metrics?.info ?? 0)}</div>
          <div className="text-[10px] text-yellow-600 dark:text-yellow-500 uppercase tracking-wider font-semibold mt-0.5">{t('metrics.minorInfo')}</div>
        </div>
      </div>

      <div>
        <h4 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3 flex items-center">
          {t('panel.detectedIssues')}
          {issuesLoading && <Loader2 className="w-3 h-3 animate-spin ml-2 text-slate-400" />}
        </h4>
        <QualityIssueList issues={issues} maxHeight="500px" />
      </div>
    </div>
  );
}
