import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Shape of the JSON stored in `PlanningDraftResponse.auditPlan` /
 * `WorkflowDetailDto.auditPlan` (Rust `AuditPlan`, snake_case serialization).
 */
interface AuditPlanDimension {
  name: string;
  name_zh: string;
  max_score: number;
  criteria: string[];
  sub_dimensions: AuditPlanDimension[] | null;
}

interface AuditPlanJson {
  mode: string;
  dimensions: AuditPlanDimension[];
  pass_threshold: number;
  generated_at: string;
  raw_principles: string;
}

export function parseAuditPlan(json: string | null): AuditPlanJson | null {
  if (!json) return null;
  try {
    const plan = JSON.parse(json) as AuditPlanJson;
    if (!Array.isArray(plan.dimensions)) return null;
    return plan;
  } catch {
    return null;
  }
}

/** Extract a leading "[RP-xxx]" point tag from a criterion string. */
function splitPointTag(criterion: string): { code: string | null; text: string } {
  const match = /^\[(RP-\d+)\]\s*(.*)$/.exec(criterion.trim());
  if (match) return { code: match[1], text: match[2] };
  return { code: null, text: criterion.trim() };
}

interface AuditPlanCardProps {
  /** JSON-serialized AuditPlan from the draft or the workflow snapshot. */
  readonly auditPlanJson: string | null;
  /** Open the card expanded (default collapsed). */
  readonly defaultExpanded?: boolean;
}

/**
 * In-conversation acceptance-rubric card (P0: the rubric was generated and
 * stored but never rendered anywhere — this makes the 评分点 visible and
 * keeps them viewable after delivery).
 */
export function AuditPlanCard({
  auditPlanJson,
  defaultExpanded = false,
}: AuditPlanCardProps) {
  const { t } = useTranslation('tasks');
  const [expanded, setExpanded] = useState(defaultExpanded);

  const plan = useMemo(() => parseAuditPlan(auditPlanJson), [auditPlanJson]);
  if (!plan) return null;

  const pointCount = plan.dimensions
    .filter((d) => d.name === 'functional_completeness')
    .reduce((n, d) => n + d.criteria.length, 0);

  return (
    <div className="rounded-lg border border-brand/30 bg-brand/5 overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="w-full flex items-center gap-half px-base py-half text-left hover:bg-brand/10 transition-colors"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 16 16"
          fill="currentColor"
          className={`size-icon-xs text-brand shrink-0 transition-transform ${expanded ? 'rotate-90' : ''}`}
        >
          <path
            fillRule="evenodd"
            d="M6.22 4.22a.75.75 0 0 1 1.06 0l3.25 3.25a.75.75 0 0 1 0 1.06l-3.25 3.25a.75.75 0 0 1-1.06-1.06L8.94 8 6.22 5.28a.75.75 0 0 1 0-1.06Z"
            clipRule="evenodd"
          />
        </svg>
        <span className="text-sm font-medium text-high">
          {t('conversation.planning.auditPlan.cardTitle')}
        </span>
        <span className="text-xs text-low">
          {t('conversation.planning.auditPlan.cardSummary', {
            points: pointCount,
            threshold: plan.pass_threshold,
          })}
        </span>
      </button>

      {expanded && (
        <div className="px-base pb-base space-y-base">
          {plan.dimensions.map((dim) => (
            <div key={dim.name}>
              <div className="flex items-center gap-half">
                <span className="text-xs font-medium text-high">
                  {dim.name_zh || dim.name}
                </span>
                <span className="text-xs text-low">
                  {t('conversation.planning.auditPlan.maxScore', {
                    max: dim.max_score,
                  })}
                </span>
              </div>
              <ul className="mt-half space-y-px">
                {dim.criteria.map((criterion) => {
                  const { code, text } = splitPointTag(criterion);
                  return (
                    <li
                      key={criterion}
                      className="text-xs text-normal flex items-start gap-half"
                    >
                      {code ? (
                        <span className="shrink-0 rounded bg-brand/15 px-half text-brand font-ibm-plex-mono">
                          {code}
                        </span>
                      ) : (
                        <span className="shrink-0 text-low">•</span>
                      )}
                      <span>{text}</span>
                    </li>
                  );
                })}
              </ul>
              {dim.sub_dimensions && dim.sub_dimensions.length > 0 && (
                <ul className="mt-half pl-base space-y-px">
                  {dim.sub_dimensions.map((sub) => (
                    <li key={sub.name} className="text-xs text-low">
                      {sub.name_zh || sub.name} ·{' '}
                      {t('conversation.planning.auditPlan.maxScore', {
                        max: sub.max_score,
                      })}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
