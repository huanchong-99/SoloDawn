import React from 'react';
import { Field, FieldLabel, FieldError } from '../../ui-new/primitives/Field';
import { cn } from '@/lib/utils';
import type { BasicConfig } from '../types';
import { useTranslation } from 'react-i18next';

const TASK_COUNT_OPTIONS = [1, 2, 3, 4];

interface Step1BasicProps {
  config: BasicConfig;
  onChange: (updates: Partial<BasicConfig>) => void;
  errors: Record<string, string>;
}

export const Step1Basic: React.FC<Step1BasicProps> = ({
  config,
  onChange,
  errors,
}) => {
  const { t } = useTranslation('workflow');

  return (
    <div className="flex flex-col gap-base">
      <Field>
        <FieldLabel>{t('step1.nameLabel')}</FieldLabel>
        <input
          type="text"
          value={config.name}
          onChange={(e) => onChange({ name: e.target.value })}
          placeholder={t('step1.namePlaceholder')}
          className={cn(
            'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
            'placeholder:text-low placeholder:opacity-80',
            'focus:outline-none focus:ring-1 focus:ring-brand',
            errors.name && 'border-error'
          )}
        />
        {errors.name && <FieldError>{t(errors.name)}</FieldError>}
      </Field>

      <Field>
        <FieldLabel>{t('step1.descriptionLabel')}</FieldLabel>
        <textarea
          value={config.description ?? ''}
          onChange={(e) => onChange({ description: e.target.value })}
          placeholder={t('step1.descriptionPlaceholder')}
          rows={3}
          className={cn(
            'w-full bg-secondary rounded-sm border px-base py-half text-base text-normal',
            'placeholder:text-low placeholder:opacity-80',
            'focus:outline-none focus:ring-1 focus:ring-brand',
            'resize-none'
          )}
        />
      </Field>

      <Field>
        <FieldLabel>{t('step1.taskCountLabel')}</FieldLabel>
        <div className="flex flex-wrap gap-base">
          {TASK_COUNT_OPTIONS.map((count) => (
            <button
              key={count}
              type="button"
              onClick={() => onChange({ taskCount: count })}
              className={cn(
                'cursor-pointer px-base py-half rounded-sm border text-base transition-colors',
                'hover:border-brand hover:text-high',
                config.taskCount === count
                  ? 'border-brand bg-brand/10 text-high'
                  : 'border-border text-normal bg-secondary'
              )}
            >
              {t('step1.taskCountOption', { count })}
            </button>
          ))}
        </div>
        <div className="mt-base flex items-center gap-base">
          <span className="text-base text-low">{t('step1.customCountLabel')}</span>
          <input
            type="number"
            min={5}
            max={10}
            value={
              config.taskCount >= 5 && config.taskCount <= 10
                ? config.taskCount
                : ''
            }
            onChange={(e) => {
              const value = Number.parseInt(e.target.value, 10);
              if (!Number.isNaN(value) && value >= 5 && value <= 10) {
                onChange({ taskCount: value });
              }
            }}
            placeholder={t('step1.customCountPlaceholder')}
            className={cn(
              'w-20 bg-secondary rounded-sm border px-base py-half text-base text-normal',
              'placeholder:text-low placeholder:opacity-80',
              'focus:outline-none focus:ring-1 focus:ring-brand'
            )}
          />
        </div>
        {errors.taskCount && <FieldError>{t(errors.taskCount)}</FieldError>}
      </Field>

      <Field>
        <FieldLabel>{t('step1.importLabel')}</FieldLabel>
        <div className="flex flex-col gap-base">
          <label className="flex items-center gap-base cursor-pointer">
            <input
              type="radio"
              name="importMode"
              checked={!config.importFromKanban}
              onChange={() => onChange({ importFromKanban: false })}
              className="size-icon-sm accent-brand"
            />
            <span className="text-base text-normal">{t('step1.importNew')}</span>
          </label>
          <label className="flex items-center gap-base cursor-pointer">
            <input
              type="radio"
              name="importMode"
              checked={config.importFromKanban}
              onChange={() => onChange({ importFromKanban: true })}
              className="size-icon-sm accent-brand"
            />
            <span className="text-base text-normal">{t('step1.importKanban')}</span>
          </label>
        </div>
      </Field>
    </div>
  );
};
