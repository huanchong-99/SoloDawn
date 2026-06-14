import { useTranslation } from 'react-i18next';

interface OrchestratorHeaderProps {
  name: string;
  status: string;
  model: string | null;
}

export function OrchestratorHeader({ name, status, model }: Readonly<OrchestratorHeaderProps>) {
  const { t } = useTranslation('workflow');
  return (
    <div className="h-16 bg-panel border-b border-border px-6 flex items-center">
      <div className="flex-1">
        <div className="text-lg font-semibold">{name}</div>
        <div className="text-xs text-low">
          {t('pipeline.orchestrator.statusLabel')} {status} | {t('pipeline.orchestrator.modelLabel')} {model ?? 'n/a'}
        </div>
      </div>
    </div>
  );
}
