import { 
  CheckCircle, 
  XOctagon, 
  AlertTriangle, 
  Loader2, 
  MinusCircle 
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { useTerminalQuality, useWorkflowQuality } from '@/hooks/useQualityReports';

export interface QualityGateStatusBadgeProps {
  status?: 'passed' | 'failed' | 'warn' | 'running' | 'none';
  onClick?: () => void;
  className?: string;
  terminalId?: string;
  workflowId?: string;
}

export function QualityGateStatusBadge({ 
  status: initialStatus,
  terminalId,
  workflowId,
  onClick, 
  className 
}: QualityGateStatusBadgeProps) {
  // If IDs are provided, try to fetch the real status periodically if it's running
  const terminalQualityQuery = useTerminalQuality(terminalId, {
    enabled: !!terminalId,
    refetchInterval: (query) => 
      query.state.data?.[0]?.status === 'running' ? 3000 : false
  });

  const workflowQualityQuery = useWorkflowQuality(workflowId, {
    enabled: !!workflowId,
  });

  let derivedStatus = initialStatus || 'none';

  if (terminalId && terminalQualityQuery.data && terminalQualityQuery.data.length > 0) {
    const latestRun = terminalQualityQuery.data[0];
    const runStatus = latestRun.gateStatus;

    if (runStatus === 'running' || runStatus === 'pending') {
      derivedStatus = 'running';
    } else if (runStatus === 'ok') {
      derivedStatus = 'passed';
    } else if (runStatus === 'error') {
      derivedStatus = 'failed';
    } else if (runStatus === 'warn') {
      derivedStatus = 'warn';
    }
  } else if (workflowId && workflowQualityQuery.data && workflowQualityQuery.data.length > 0) {
    const latestRun = workflowQualityQuery.data[0];
    const runStatus = latestRun.gateStatus;

    if (runStatus === 'running' || runStatus === 'pending') {
      derivedStatus = 'running';
    } else if (runStatus === 'ok') {
      derivedStatus = 'passed';
    } else if (runStatus === 'error') {
      derivedStatus = 'failed';
    } else if (runStatus === 'warn') {
      derivedStatus = 'warn';
    }
  }

  const { icon: Icon, label, variantClasses } = {
    passed: {
      icon: CheckCircle,
      label: 'Quality Gate Passed',
      variantClasses: 'bg-green-100 text-green-800 border-green-200 dark:bg-green-900/30 dark:text-green-400 dark:border-green-800',
    },
    failed: {
      icon: XOctagon,
      label: 'Quality Gate Failed',
      variantClasses: 'bg-red-100 text-red-800 border-red-200 dark:bg-red-900/30 dark:text-red-400 dark:border-red-800',
    },
    warn: {
      icon: AlertTriangle,
      label: 'Quality Gate Warning',
      variantClasses: 'bg-amber-100 text-amber-800 border-amber-200 dark:bg-amber-900/30 dark:text-amber-400 dark:border-amber-800',
    },
    running: {
      icon: Loader2,
      label: 'Analysing Quality...',
      variantClasses: 'bg-blue-100 text-blue-800 border-blue-200 dark:bg-blue-900/30 dark:text-blue-400 dark:border-blue-800',
    },
    none: {
      icon: MinusCircle,
      label: 'No Quality Gate',
      variantClasses: 'bg-slate-100 text-slate-800 border-slate-200 dark:bg-slate-800 dark:text-slate-400 dark:border-slate-700',
    },
  }[derivedStatus];

  if (derivedStatus === 'none' && !initialStatus) {
    return null; // hide if no data and no initial requested
  }

  return (
    <Badge 
      variant="outline" 
      className={cn(
        'cursor-pointer transition-colors shadow-sm', 
        variantClasses, 
        className
      )}
      onClick={onClick}
    >
      <Icon className={cn("w-3.5 h-3.5 mr-1.5", derivedStatus === 'running' && "animate-spin")} />
      {label}
    </Badge>
  );
}
