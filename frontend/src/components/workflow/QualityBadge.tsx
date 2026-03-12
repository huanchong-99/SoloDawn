import { Shield, ShieldCheck, ShieldAlert, ShieldX, Loader2 } from 'lucide-react';
import { StatusPill } from '@/components/ui-new/primitives/StatusPill';

type GateStatus = 'pending' | 'running' | 'ok' | 'warn' | 'error' | 'skipped';

interface QualityBadgeProps {
  readonly gateStatus: GateStatus | string;
  readonly totalIssues?: number;
  readonly blockingIssues?: number;
  readonly mode?: string;
  readonly className?: string;
}

function statusToTone(status: string) {
  switch (status) {
    case 'ok':
      return 'success' as const;
    case 'warn':
      return 'warning' as const;
    case 'error':
      return 'danger' as const;
    case 'running':
    case 'pending':
      return 'info' as const;
    case 'skipped':
    default:
      return 'neutral' as const;
  }
}

function statusToIcon(status: string) {
  switch (status) {
    case 'ok':
      return ShieldCheck;
    case 'warn':
      return ShieldAlert;
    case 'error':
      return ShieldX;
    case 'running':
    case 'pending':
      return Loader2;
    default:
      return Shield;
  }
}

function statusToLabel(status: string, blockingIssues?: number) {
  switch (status) {
    case 'ok':
      return 'Passed';
    case 'warn':
      return blockingIssues ? `${blockingIssues} warnings` : 'Warning';
    case 'error':
      return blockingIssues ? `${blockingIssues} blocking` : 'Failed';
    case 'running':
      return 'Scanning...';
    case 'pending':
      return 'Pending';
    case 'skipped':
      return 'Skipped';
    default:
      return status;
  }
}

export function QualityBadge({
  gateStatus,
  blockingIssues,
  className,
}: Readonly<QualityBadgeProps>) {
  return (
    <StatusPill
      tone={statusToTone(gateStatus)}
      size="sm"
      icon={statusToIcon(gateStatus)}
      label={statusToLabel(gateStatus, blockingIssues)}
      className={className}
    />
  );
}
