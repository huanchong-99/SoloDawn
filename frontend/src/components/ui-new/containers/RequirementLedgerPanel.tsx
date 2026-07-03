import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { RequirementItemResponse } from '@/lib/api';
import {
  parseCapsule,
  useDeleteRequirementItem,
  useRequirementItems,
  useUpdateRequirementItem,
} from '@/hooks/useRequirementItems';

const STATUS_DOT: Record<RequirementItemResponse['status'], string> = {
  pending: 'bg-secondary border border-default',
  delivered: 'bg-success',
  regressed: 'bg-error',
};

function CapsuleBlock({ item }: { readonly item: RequirementItemResponse }) {
  const { t } = useTranslation('tasks');
  const capsule = parseCapsule(item);
  if (!capsule) return null;
  const rows: Array<[string, string]> = [
    [t('conversation.planning.ledger.capsuleBuilt'), capsule.built],
    [t('conversation.planning.ledger.capsuleLivesWhere'), capsule.livesWhere],
    [t('conversation.planning.ledger.capsuleDecisions'), capsule.decisions],
    [
      t('conversation.planning.ledger.capsuleExtensionNotes'),
      capsule.extensionNotes,
    ],
  ];
  return (
    <div className="mt-half rounded bg-secondary/60 px-half py-half space-y-px">
      {rows
        .filter(([, value]) => value.trim().length > 0)
        .map(([label, value]) => (
          <p key={label} className="text-xs text-low">
            <span className="text-normal">{label}:</span> {value}
          </p>
        ))}
      {item.provenanceCommits && (
        <p className="text-xs text-low">
          <span className="text-normal">
            {t('conversation.planning.ledger.capsuleProvenance')}:
          </span>{' '}
          {item.provenanceCommits}
        </p>
      )}
    </div>
  );
}

function LedgerRow({
  item,
  projectId,
}: {
  readonly item: RequirementItemResponse;
  readonly projectId: string;
}) {
  const { t } = useTranslation('tasks');
  const [showCapsule, setShowCapsule] = useState(false);
  const [editing, setEditing] = useState(false);
  const [draftText, setDraftText] = useState(item.text);
  const updateMutation = useUpdateRequirementItem();
  const deleteMutation = useDeleteRequirementItem();

  const isPending = item.status === 'pending';
  const hasCapsule = !!item.contextCapsule;

  const handleSave = useCallback(() => {
    const text = draftText.trim();
    if (!text || text === item.text) {
      setEditing(false);
      setDraftText(item.text);
      return;
    }
    updateMutation.mutate(
      { projectId, itemId: item.id, text },
      { onSettled: () => setEditing(false) }
    );
  }, [draftText, item.id, item.text, projectId, updateMutation]);

  return (
    <li className="rounded border border-default bg-secondary/40 px-half py-half">
      <div className="flex items-start gap-half">
        <span
          className={`mt-1 size-1.5 rounded-full shrink-0 ${STATUS_DOT[item.status]}`}
          title={t(`conversation.planning.ledger.status.${item.status}`)}
        />
        <span className="text-xs font-ibm-plex-mono text-brand shrink-0">
          {item.pointCode}
        </span>
        {editing ? (
          <textarea
            value={draftText}
            onChange={(e) => setDraftText(e.target.value)}
            rows={2}
            className="flex-1 text-xs bg-secondary rounded border border-default px-half py-px text-normal focus:outline-none focus:ring-1 focus:ring-brand"
          />
        ) : (
          <span className="flex-1 text-xs text-normal break-words">
            {item.text}
          </span>
        )}
      </div>

      <div className="mt-half flex items-center gap-half pl-3">
        {hasCapsule && (
          <button
            type="button"
            onClick={() => setShowCapsule((v) => !v)}
            className="text-xs text-low hover:text-high transition-colors"
          >
            {showCapsule
              ? t('conversation.planning.ledger.hideCapsule')
              : t('conversation.planning.ledger.showCapsule')}
          </button>
        )}
        {isPending && !editing && (
          <button
            type="button"
            onClick={() => setEditing(true)}
            className="text-xs text-low hover:text-high transition-colors"
          >
            {t('conversation.planning.ledger.edit')}
          </button>
        )}
        {isPending && editing && (
          <>
            <button
              type="button"
              onClick={handleSave}
              disabled={updateMutation.isPending}
              className="text-xs text-brand hover:text-brand/80 transition-colors disabled:opacity-50"
            >
              {t('conversation.planning.ledger.save')}
            </button>
            <button
              type="button"
              onClick={() => {
                setEditing(false);
                setDraftText(item.text);
              }}
              className="text-xs text-low hover:text-high transition-colors"
            >
              {t('conversation.planning.ledger.cancel')}
            </button>
          </>
        )}
        {isPending && !editing && (
          <button
            type="button"
            onClick={() =>
              deleteMutation.mutate({ projectId, itemId: item.id })
            }
            disabled={deleteMutation.isPending}
            className="text-xs text-low hover:text-error transition-colors disabled:opacity-50"
          >
            {t('conversation.planning.ledger.delete')}
          </button>
        )}
      </div>

      {showCapsule && <CapsuleBlock item={item} />}
    </li>
  );
}

interface RequirementLedgerPanelProps {
  readonly projectId: string | null;
}

/**
 * Project-scoped requirement ledger (需求清单 / 评分点账本) side panel.
 * Mirrors AuditDocPanel's collapsed-tab pattern. Points accumulate across
 * rounds; delivered points expose their compressed context capsule.
 */
export function RequirementLedgerPanel({
  projectId,
}: RequirementLedgerPanelProps) {
  const { t } = useTranslation('tasks');
  const [expanded, setExpanded] = useState(false);
  const { data: items } = useRequirementItems(projectId);

  // No ledger yet (nothing confirmed on this project): stay invisible.
  if (!projectId || !items || items.length === 0) return null;

  if (!expanded) {
    return (
      <button
        type="button"
        onClick={() => setExpanded(true)}
        className="shrink-0 w-7 bg-secondary border-l border-default flex flex-col items-center justify-center gap-1 cursor-pointer hover:bg-panel transition-colors"
        title={t('conversation.planning.ledger.panelTab')}
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 16 16"
          fill="currentColor"
          className="size-icon-sm text-low"
        >
          <path
            fillRule="evenodd"
            d="M2.5 3A1.5 1.5 0 0 0 1 4.5v7A1.5 1.5 0 0 0 2.5 13h11a1.5 1.5 0 0 0 1.5-1.5v-7A1.5 1.5 0 0 0 13.5 3h-11ZM4 6a.75.75 0 0 0 0 1.5h.5A.75.75 0 0 0 5.25 6H4Zm2.75 0a.75.75 0 0 0 0 1.5H12A.75.75 0 0 0 12 6H6.75ZM4 9a.75.75 0 0 0 0 1.5h.5A.75.75 0 0 0 5.25 9H4Zm2.75 0a.75.75 0 0 0 0 1.5H12A.75.75 0 0 0 12 9H6.75Z"
            clipRule="evenodd"
          />
        </svg>
        <span
          className="text-xs text-low"
          style={{ writingMode: 'vertical-rl', textOrientation: 'mixed' }}
        >
          {t('conversation.planning.ledger.panelTab')}
        </span>
      </button>
    );
  }

  const delivered = items.filter((i) => i.status === 'delivered').length;

  return (
    <div className="shrink-0 w-[280px] bg-panel border-l border-default flex flex-col overflow-hidden">
      <div className="flex items-center justify-between px-base py-half border-b border-default">
        <span className="text-sm font-medium text-high">
          {t('conversation.planning.ledger.panelTitle')}
        </span>
        <span className="text-xs text-low">
          {t('conversation.planning.ledger.progress', {
            delivered,
            total: items.length,
          })}
        </span>
        <button
          type="button"
          onClick={() => setExpanded(false)}
          className="text-low hover:text-high transition-colors p-half rounded"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 16 16"
            fill="currentColor"
            className="size-icon-xs"
          >
            <path d="M5.28 4.22a.75.75 0 0 0-1.06 1.06L6.94 8l-2.72 2.72a.75.75 0 1 0 1.06 1.06L8 9.06l2.72 2.72a.75.75 0 1 0 1.06-1.06L9.06 8l2.72-2.72a.75.75 0 0 0-1.06-1.06L8 6.94 5.28 4.22Z" />
          </svg>
        </button>
      </div>

      <ul className="flex-1 overflow-y-auto p-base space-y-half">
        {items.map((item) => (
          <LedgerRow key={item.id} item={item} projectId={projectId} />
        ))}
      </ul>
    </div>
  );
}
