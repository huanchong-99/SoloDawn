import { Shield, ShieldCheck, ShieldAlert, ShieldX, Loader2 } from 'lucide-react';
import { StatusPill } from '@/components/ui-new/primitives/StatusPill';
import { useTranslation } from 'react-i18next';

export type GateStatus = 'pending' | 'running' | 'ok' | 'warn' | 'error' | 'skipped';

interface QualityBadgeProps {
  readonly gateStatus: GateStatus;
  readonly blockingIssues?: number;
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

export function QualityBadge({
  gateStatus,
  blockingIssues,
  className,
}: Readonly<QualityBadgeProps>) {
  const { t } = useTranslation('quality');

  function statusToLabel(status: string, blocking?: number) {
    switch (status) {
      case 'ok':
        return t('status.ok');
      case 'warn':
        return blocking ? t('status.warnCount', { count: blocking }) : t('status.warn');
      case 'error':
        return blocking ? t('status.errorCount', { count: blocking }) : t('status.error');
      case 'running':
        return t('status.running');
      case 'pending':
        return t('status.pending');
      case 'skipped':
        return t('status.skipped');
      default:
        return status;
    }
  }

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
