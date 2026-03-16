import { useTranslation } from 'react-i18next';
import { ChatTeardropDotsIcon, EyeIcon, EyeSlashIcon } from '@phosphor-icons/react';
import { useState } from 'react';

import { cn } from '@/lib/utils';

export interface SetupWizardStep4IntegrationsProps {
  feishuEnabled: boolean;
  feishuAppId: string;
  feishuAppSecret: string;
  onFeishuEnabledChange: (enabled: boolean) => void;
  onFeishuAppIdChange: (value: string) => void;
  onFeishuAppSecretChange: (value: string) => void;
  onNext: () => void;
  onSkip: () => void;
}

export function SetupWizardStep4Integrations({
  feishuEnabled,
  feishuAppId,
  feishuAppSecret,
  onFeishuEnabledChange,
  onFeishuAppIdChange,
  onFeishuAppSecretChange,
  onNext,
  onSkip,
}: Readonly<SetupWizardStep4IntegrationsProps>) {
  const { t } = useTranslation(['setup']);
  const [secretVisible, setSecretVisible] = useState(false);

  return (
    <div className="flex flex-col items-center justify-center max-w-lg mx-auto space-y-double">
      <div className="text-center space-y-base">
        <h1 className="text-high text-xl font-medium">
          {t('setup:wizard.integrations.title')}
        </h1>
        <p className="text-low text-base leading-relaxed">
          {t('setup:wizard.integrations.subtitle')}
        </p>
      </div>

      {/* Feishu Integration Card */}
      <div className="w-full rounded border border-border bg-secondary p-base space-y-base">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-base">
            <ChatTeardropDotsIcon
              className="size-icon-md text-brand"
              weight="duotone"
            />
            <div className="space-y-0.5">
              <span className="text-high text-base font-medium">
                {t('setup:wizard.integrations.feishu.label')}
              </span>
              <p className="text-low text-sm leading-snug">
                {t('setup:wizard.integrations.feishu.description')}
              </p>
            </div>
          </div>

          {/* Toggle switch */}
          <button
            type="button"
            role="switch"
            aria-checked={feishuEnabled}
            onClick={() => onFeishuEnabledChange(!feishuEnabled)}
            className={cn(
              'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors',
              'focus:outline-none focus:ring-1 focus:ring-brand focus:ring-offset-1',
              feishuEnabled ? 'bg-brand' : 'bg-panel'
            )}
          >
            <span
              className={cn(
                'inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform',
                feishuEnabled ? 'translate-x-[18px]' : 'translate-x-[3px]'
              )}
            />
          </button>
        </div>

        {/* Config fields - animated reveal */}
        <div
          className={cn(
            'overflow-hidden transition-all duration-300 ease-in-out',
            feishuEnabled
              ? 'max-h-60 opacity-100'
              : 'max-h-0 opacity-0'
          )}
        >
          <div className="space-y-base pt-base border-t border-border">
            <div className="space-y-half">
              <label
                htmlFor="setup-feishu-app-id"
                className="text-normal text-base"
              >
                {t('setup:wizard.integrations.feishu.appIdLabel')}
              </label>
              <input
                id="setup-feishu-app-id"
                type="text"
                value={feishuAppId}
                onChange={(e) => onFeishuAppIdChange(e.target.value)}
                placeholder={t(
                  'setup:wizard.integrations.feishu.appIdPlaceholder'
                )}
                className={cn(
                  'w-full rounded border border-border bg-secondary',
                  'px-base py-base text-base text-normal',
                  'placeholder:text-low',
                  'focus:outline-none focus:ring-1 focus:ring-brand'
                )}
              />
            </div>

            <div className="space-y-half">
              <label
                htmlFor="setup-feishu-app-secret"
                className="text-normal text-base"
              >
                {t('setup:wizard.integrations.feishu.appSecretLabel')}
              </label>
              <div className="relative">
                <input
                  id="setup-feishu-app-secret"
                  type={secretVisible ? 'text' : 'password'}
                  value={feishuAppSecret}
                  onChange={(e) => onFeishuAppSecretChange(e.target.value)}
                  placeholder={t(
                    'setup:wizard.integrations.feishu.appSecretPlaceholder'
                  )}
                  className={cn(
                    'w-full rounded border border-border bg-secondary',
                    'px-base py-base pr-8 text-base text-normal',
                    'placeholder:text-low',
                    'focus:outline-none focus:ring-1 focus:ring-brand'
                  )}
                />
                <button
                  type="button"
                  onClick={() => setSecretVisible((v) => !v)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-low hover:text-normal transition-colors"
                  tabIndex={-1}
                >
                  {secretVisible ? (
                    <EyeSlashIcon className="size-icon-xs" weight="regular" />
                  ) : (
                    <EyeIcon className="size-icon-xs" weight="regular" />
                  )}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Skip hint */}
      <p className="text-low text-sm text-center">
        {t('setup:wizard.integrations.skipHint')}
      </p>

      {/* Action buttons */}
      <div className="w-full flex flex-col items-center gap-base pt-base">
        <button
          type="button"
          onClick={onNext}
          className={cn(
            'w-full bg-brand text-white px-double py-base rounded font-medium text-base',
            'hover:opacity-90 transition-opacity',
            'focus:outline-none focus:ring-1 focus:ring-brand focus:ring-offset-1'
          )}
        >
          {t('setup:wizard.welcome.continueButton')}
        </button>
        <button
          type="button"
          onClick={onSkip}
          className="text-low text-sm hover:text-normal underline transition-colors"
        >
          {t('setup:wizard.welcome.skipButton')}
        </button>
      </div>
    </div>
  );
}
