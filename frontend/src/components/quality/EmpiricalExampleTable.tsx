import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/utils';
import type { ExampleResult } from 'shared/types';

/**
 * Read-only empirical example table (Kind / Snippet / Expected / Actual) shared
 * by the rule-authoring dialog and the confirm-dialog evidence panel. Renders
 * only the table itself; callers own the empty-state copy and the surrounding
 * section. Extracted to remove a duplicated block across both consumers.
 */
export function EmpiricalExampleTable({
  perExample,
}: Readonly<{ perExample: ExampleResult[] }>) {
  const { t } = useTranslation(['settings']);
  const matchLabel = (matched: boolean) =>
    matched
      ? t('settings:ruleAuthoring.match', { defaultValue: 'match' })
      : t('settings:ruleAuthoring.noMatch', { defaultValue: 'no match' });
  return (
    <div className="overflow-x-auto rounded border border-border">
      <table className="w-full text-xs">
        <thead>
          <tr className="text-left text-low">
            <th className="px-half py-half font-semibold">
              {t('settings:ruleAuthoring.colKind', { defaultValue: 'Kind' })}
            </th>
            <th className="px-half py-half font-semibold">
              {t('settings:ruleAuthoring.colSnippet', {
                defaultValue: 'Snippet',
              })}
            </th>
            <th className="px-half py-half font-semibold">
              {t('settings:ruleAuthoring.colExpected', {
                defaultValue: 'Expected',
              })}
            </th>
            <th className="px-half py-half font-semibold">
              {t('settings:ruleAuthoring.colActual', {
                defaultValue: 'Actual',
              })}
            </th>
          </tr>
        </thead>
        <tbody>
          {perExample.map((ex, i) => (
            <tr
              key={`${ex.kind}-${i}-${ex.snippet.slice(0, 16)}`}
              className={cn(
                'border-t border-border align-top',
                !ex.passed && 'bg-error/10'
              )}
            >
              <td className="px-half py-half text-normal">{ex.kind}</td>
              <td className="px-half py-half">
                <pre className="max-w-[20rem] overflow-x-auto whitespace-pre-wrap font-ibm-plex-mono text-normal">
                  {ex.snippet}
                </pre>
              </td>
              <td
                className={cn(
                  'px-half py-half',
                  !ex.passed ? 'text-error font-semibold' : 'text-normal'
                )}
              >
                {matchLabel(ex.expectedMatch)}
              </td>
              <td
                className={cn(
                  'px-half py-half',
                  !ex.passed ? 'text-error font-semibold' : 'text-normal'
                )}
              >
                {matchLabel(ex.actualMatch)}
                {ex.matchCount > 0 ? ` (${ex.matchCount})` : ''}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default EmpiricalExampleTable;
