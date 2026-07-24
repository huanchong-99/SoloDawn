import { useTranslation } from 'react-i18next';
import { Play } from 'lucide-react';
import { Button } from '@/components/ui-new/primitives/Button';

interface OrchestratorHeaderProps {
  name: string;
  status: string;
  model: string | null;
  /**
   * True when the workflow is `paused` and can be resumed. A workflow lands
   * here either because the user paused it or because restart recovery
   * auto-paused it (the backend never auto-fails on a restart artifact), so
   * this header is the only place the user can get execution going again.
   */
  canResume?: boolean;
  isResuming?: boolean;
  onResume?: () => void;
}

export function OrchestratorHeader({
  name,
  status,
  model,
  canResume = false,
  isResuming = false,
  onResume,
}: Readonly<OrchestratorHeaderProps>) {
  const { t } = useTranslation('workflow');
  const showResume = canResume && Boolean(onResume);

  return (
    <div className="h-16 bg-panel border-b border-border px-6 flex items-center gap-4">
      <div className="flex-1 min-w-0">
        <div className="text-lg font-semibold">{name}</div>
        <div className="text-xs text-low">
          {t('pipeline.orchestrator.statusLabel')} {status} | {t('pipeline.orchestrator.modelLabel')} {model ?? 'n/a'}
        </div>
      </div>
      {showResume && (
        <div className="flex items-center gap-3">
          <span className="hidden text-xs text-low sm:inline">
            {t('pipeline.orchestrator.pausedHint')}
          </span>
          <Button size="sm" onClick={onResume} disabled={isResuming}>
            <Play className="w-3.5 h-3.5" />
            {isResuming
              ? t('pipeline.orchestrator.resuming')
              : t('pipeline.orchestrator.resume')}
          </Button>
        </div>
      )}
    </div>
  );
}
