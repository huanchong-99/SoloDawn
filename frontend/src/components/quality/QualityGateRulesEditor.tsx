import { Plus, Trash2, AlertTriangle } from 'lucide-react';
import type {
  QualityGateConfig,
  QualityGateMode,
  GateDefinition,
  ConditionConfig,
  ProvidersConfig,
  SonarConfig,
  MetricKey,
} from 'shared/types';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export interface QualityGateRulesEditorProps {
  value: QualityGateConfig;
  defaults: QualityGateConfig;
  /** Picker source from GET /quality/policy/metrics — the closed MetricKey enum (sentinel excluded). */
  metricOptions: MetricKey[];
  onChange: (next: QualityGateConfig) => void;
  readOnly?: boolean;
  /** Field-level validation errors to surface (one human-readable string per offending condition). */
  errors?: string[];
}

const MODES: QualityGateMode[] = ['off', 'shadow', 'warn', 'enforce'];
const OPERATORS = ['GT', 'LT'] as const;

/** The exact 11 provider toggles from crates/quality/src/config.rs (ProvidersConfig). */
const PROVIDER_KEYS: ReadonlyArray<keyof ProvidersConfig> = [
  'rust',
  'frontend',
  'repo',
  'security',
  'sonar',
  'builtin_rust',
  'builtin_frontend',
  'builtin_common',
  'coverage',
  'completeness',
  'delivery_readiness',
];

type GateKey = 'terminal_gate' | 'branch_gate' | 'repo_gate';

const GATE_SECTIONS: ReadonlyArray<{ key: GateKey; title: string }> = [
  { key: 'terminal_gate', title: 'Terminal Gate' },
  { key: 'branch_gate', title: 'Branch Gate' },
  { key: 'repo_gate', title: 'Repo Gate' },
];

const inputCls =
  'px-2 py-1 bg-white dark:bg-slate-900 rounded border border-slate-200 dark:border-slate-700 text-sm text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-60 disabled:cursor-not-allowed';

const labelCls = 'text-xs font-semibold text-slate-500 uppercase tracking-wider';

export function QualityGateRulesEditor({
  value,
  defaults,
  metricOptions,
  onChange,
  readOnly = false,
  errors = [],
}: Readonly<QualityGateRulesEditorProps>) {
  // ---- pure helpers: build the next config and bubble it up via onChange ----
  const patch = (partial: Partial<QualityGateConfig>) => {
    onChange({ ...value, ...partial });
  };

  const patchGate = (key: GateKey, next: GateDefinition) => {
    patch({ [key]: next } as Partial<QualityGateConfig>);
  };

  const addCondition = (key: GateKey) => {
    const gate = value[key];
    // Default the metric to the first available picker option so it never holds a free-text value.
    const metric = (metricOptions[0] ?? '') as MetricKey;
    const nextCond: ConditionConfig = { metric, operator: 'GT', threshold: '0' };
    patchGate(key, { ...gate, conditions: [...gate.conditions, nextCond] });
  };

  const updateCondition = (key: GateKey, index: number, partial: Partial<ConditionConfig>) => {
    const gate = value[key];
    const conditions = gate.conditions.map((c, i) => (i === index ? { ...c, ...partial } : c));
    patchGate(key, { ...gate, conditions });
  };

  const removeCondition = (key: GateKey, index: number) => {
    const gate = value[key];
    patchGate(key, { ...gate, conditions: gate.conditions.filter((_, i) => i !== index) });
  };

  const patchProviders = (partial: Partial<ProvidersConfig>) => {
    patch({ providers: { ...value.providers, ...partial } });
  };

  const patchSonar = (partial: Partial<SonarConfig>) => {
    patch({ sonar: { ...value.sonar, ...partial } });
  };

  return (
    <div className="flex flex-col gap-6 text-slate-900 dark:text-slate-100">
      {/* Validation errors */}
      {errors.length > 0 && (
        <div className="p-3 bg-red-50 dark:bg-red-950/20 border border-red-100 dark:border-red-900/30 rounded-md">
          <div className="flex items-center gap-2 text-sm font-semibold text-red-700 dark:text-red-400 mb-1">
            <AlertTriangle className="w-4 h-4" />
            <span>Invalid quality policy</span>
          </div>
          <ul className="list-disc pl-6 text-xs text-red-600 dark:text-red-400 space-y-0.5">
            {errors.map((err) => (
              <li key={err}>{err}</li>
            ))}
          </ul>
        </div>
      )}

      {/* Mode */}
      <section className="flex flex-col gap-2">
        <span className={labelCls}>Mode</span>
        <select
          className={cn(inputCls, 'w-48')}
          value={value.mode}
          disabled={readOnly}
          onChange={(e) => patch({ mode: e.target.value as QualityGateMode })}
        >
          {MODES.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </select>
      </section>

      {/* Gate sections */}
      {GATE_SECTIONS.map(({ key, title }) => {
        const gate = value[key];
        return (
          <section
            key={key}
            className="border border-slate-200 dark:border-slate-800 rounded-lg p-4 bg-slate-50 dark:bg-slate-900/40"
          >
            <div className="flex items-center justify-between mb-3">
              <h4 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{title}</h4>
              <Button
                type="button"
                variant="outline"
                size="xs"
                disabled={readOnly}
                onClick={() => addCondition(key)}
              >
                <Plus className="w-3.5 h-3.5 mr-1" />
                Add condition
              </Button>
            </div>

            {gate.conditions.length === 0 ? (
              <p className="text-xs text-slate-400 italic py-2">No conditions.</p>
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left">
                    <th className={cn(labelCls, 'pb-2 pr-2 font-semibold')}>Metric</th>
                    <th className={cn(labelCls, 'pb-2 pr-2 font-semibold w-24')}>Operator</th>
                    <th className={cn(labelCls, 'pb-2 pr-2 font-semibold w-32')}>Threshold</th>
                    <th className="w-10" />
                  </tr>
                </thead>
                <tbody>
                  {gate.conditions.map((cond, index) => (
                    <tr
                      key={`${key}-${index}-${cond.metric}-${cond.operator}`}
                      className="border-t border-slate-200 dark:border-slate-800"
                    >
                      <td className="py-2 pr-2">
                        <select
                          className={cn(inputCls, 'w-full')}
                          value={cond.metric}
                          disabled={readOnly}
                          onChange={(e) =>
                            updateCondition(key, index, { metric: e.target.value as MetricKey })
                          }
                        >
                          {/* Options come ONLY from metricOptions ⇒ enforces the closed enum; no free text. */}
                          {metricOptions.map((m) => (
                            <option key={m} value={m}>
                              {m}
                            </option>
                          ))}
                          {/* Surface a stale/out-of-range metric so the select still shows it. */}
                          {!metricOptions.includes(cond.metric) && cond.metric && (
                            <option value={cond.metric}>{cond.metric} (unknown)</option>
                          )}
                        </select>
                      </td>
                      <td className="py-2 pr-2">
                        <select
                          className={cn(inputCls, 'w-full')}
                          value={cond.operator}
                          disabled={readOnly}
                          onChange={(e) => updateCondition(key, index, { operator: e.target.value })}
                        >
                          {OPERATORS.map((op) => (
                            <option key={op} value={op}>
                              {op}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td className="py-2 pr-2">
                        <input
                          type="text"
                          inputMode="decimal"
                          className={cn(inputCls, 'w-full')}
                          value={cond.threshold}
                          disabled={readOnly}
                          onChange={(e) => updateCondition(key, index, { threshold: e.target.value })}
                        />
                      </td>
                      <td className="py-2 text-right">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="w-7 h-7 text-slate-400 hover:text-red-500"
                          disabled={readOnly}
                          aria-label="Delete condition"
                          onClick={() => removeCondition(key, index)}
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>
        );
      })}

      {/* Providers — exactly the 11 ProvidersConfig toggles */}
      <section className="flex flex-col gap-2">
        <span className={labelCls}>Providers</span>
        <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
          {PROVIDER_KEYS.map((pk) => (
            <label
              key={pk}
              className={cn(
                'flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 select-none',
                readOnly ? 'cursor-not-allowed' : 'cursor-pointer'
              )}
            >
              <input
                type="checkbox"
                className="w-4 h-4 rounded border-slate-300 dark:border-slate-600 text-blue-600 focus:ring-blue-500 disabled:opacity-60"
                checked={value.providers[pk]}
                disabled={readOnly}
                onChange={(e) => patchProviders({ [pk]: e.target.checked } as Partial<ProvidersConfig>)}
              />
              <span className="font-mono">{pk}</span>
            </label>
          ))}
        </div>
      </section>

      {/* SonarQube */}
      <section className="flex flex-col gap-3 border border-slate-200 dark:border-slate-800 rounded-lg p-4">
        <span className={labelCls}>SonarQube</span>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          <label className="flex flex-col gap-1">
            <span className="text-xs text-slate-500">Host URL</span>
            <input
              type="text"
              className={inputCls}
              placeholder="https://sonar.example.com"
              value={value.sonar.host_url}
              disabled={readOnly}
              onChange={(e) => patchSonar({ host_url: e.target.value })}
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs text-slate-500">Project Key</span>
            <input
              type="text"
              className={inputCls}
              placeholder="my-project"
              value={value.sonar.project_key}
              disabled={readOnly}
              onChange={(e) => patchSonar({ project_key: e.target.value })}
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs text-slate-500">Token</span>
            <input
              type="password"
              autoComplete="off"
              className={inputCls}
              placeholder="••••••••"
              value={value.sonar.token ?? ''}
              disabled={readOnly}
              onChange={(e) => patchSonar({ token: e.target.value === '' ? null : e.target.value })}
            />
          </label>
        </div>
      </section>

      {/* defaults is accepted per the shared editor contract (parents use it for reset affordances). */}
      <span className="sr-only" aria-hidden="true" data-defaults-mode={defaults.mode} />
    </div>
  );
}
