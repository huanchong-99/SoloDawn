import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Wrench, MessageSquare } from 'lucide-react';
import { cn } from '@/lib/utils';

export type ProductMode = 'manual' | 'orchestrated';

function resolveActiveMode(pathname: string): ProductMode {
  if (
    pathname.startsWith('/workspaces') ||
    pathname.startsWith('/orchestrate')
  ) {
    return 'orchestrated';
  }
  return 'manual';
}

interface ProductModeSwitchProps {
  className?: string;
}

export function ProductModeSwitch({
  className,
}: Readonly<ProductModeSwitchProps>) {
  const { t } = useTranslation('workflow');
  const navigate = useNavigate();
  const { pathname } = useLocation();

  const active = resolveActiveMode(pathname);

  const modes = [
    {
      id: 'manual' as const,
      label: t('modeSwitch.manual'),
      icon: Wrench,
      target: '/board',
    },
    {
      id: 'orchestrated' as const,
      label: t('modeSwitch.orchestrated'),
      icon: MessageSquare,
      target: '/workspaces/create',
    },
  ];

  return (
    <div className={cn('flex items-center rounded border border-border', className)}>
      {modes.map((mode) => {
        const Icon = mode.icon;
        const isActive = active === mode.id;
        return (
          <button
            key={mode.id}
            type="button"
            onClick={() => {
              if (!isActive) {
                navigate(mode.target);
              }
            }}
            className={cn(
              'flex items-center gap-1 px-2.5 py-1 text-xs font-medium transition-colors',
              'first:rounded-l last:rounded-r',
              isActive
                ? 'bg-brand/15 text-brand'
                : 'text-low hover:text-high hover:bg-secondary'
            )}
            title={mode.label}
          >
            <Icon className="w-3.5 h-3.5" />
            {mode.label}
          </button>
        );
      })}
    </div>
  );
}
