import { useState, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useUploadAuditDoc,
  useDeleteAuditDoc,
} from '@/hooks/usePlanningDraft';

interface AuditDocPanelProps {
  readonly draftId: string | null;
  readonly draftStatus: string | undefined;
  readonly auditDocPath: string | null;
  readonly auditMode: string;
  readonly onRetainBuiltinChange: (value: boolean) => void;
  readonly retainBuiltin: boolean;
}

const ACCEPTED_EXTENSIONS = '.md,.txt,.pdf,.docx';
const READ_ONLY_STATUSES = new Set(['confirmed', 'materialized', 'cancelled']);

export function AuditDocPanel({
  draftId,
  draftStatus,
  auditDocPath,
  auditMode,
  onRetainBuiltinChange,
  retainBuiltin,
}: AuditDocPanelProps) {
  const { t } = useTranslation('tasks');
  const [expanded, setExpanded] = useState(false);
  const [isDragOver, setIsDragOver] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const uploadMutation = useUploadAuditDoc();
  const deleteMutation = useDeleteAuditDoc();

  const isReadOnly = READ_ONLY_STATUSES.has(draftStatus ?? '');
  const hasDoc = !!auditDocPath;

  const fileName = auditDocPath
    ? auditDocPath.split('/').pop() ?? auditDocPath
    : null;

  const handleFileSelect = useCallback(
    (file: File) => {
      if (!draftId || isReadOnly) return;
      uploadMutation.mutate({ draftId, file });
    },
    [draftId, isReadOnly, uploadMutation]
  );

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) handleFileSelect(file);
      // Reset input so the same file can be re-selected
      if (fileInputRef.current) fileInputRef.current.value = '';
    },
    [handleFileSelect]
  );

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (!isReadOnly) setIsDragOver(true);
    },
    [isReadOnly]
  );

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDragOver(false);
      if (isReadOnly) return;
      const file = e.dataTransfer.files[0];
      if (file) handleFileSelect(file);
    },
    [isReadOnly, handleFileSelect]
  );

  const handleRemove = useCallback(() => {
    if (!draftId || isReadOnly) return;
    deleteMutation.mutate(draftId);
  }, [draftId, isReadOnly, deleteMutation]);

  // --- Collapsed tab ---
  if (!expanded) {
    return (
      <button
        type="button"
        onClick={() => setExpanded(true)}
        className="shrink-0 w-7 bg-secondary border-l border-default flex flex-col items-center justify-center gap-1 cursor-pointer hover:bg-panel transition-colors"
        title={t('conversation.planning.auditDoc.panelTab')}
      >
        {/* Document icon */}
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 16 16"
          fill="currentColor"
          className="size-3.5 text-low"
        >
          <path d="M4 1.75A.75.75 0 0 1 4.75 1h4.914a.75.75 0 0 1 .53.22l3.586 3.586a.75.75 0 0 1 .22.53v8.914a.75.75 0 0 1-.75.75H4.75a.75.75 0 0 1-.75-.75V1.75Zm5.25 1.5v2.5a.75.75 0 0 0 .75.75h2.5L9.25 3.25Z" />
        </svg>
        {/* Vertical text */}
        <span
          className="text-xs text-low"
          style={{ writingMode: 'vertical-rl', textOrientation: 'mixed' }}
        >
          {t('conversation.planning.auditDoc.panelTab')}
        </span>
      </button>
    );
  }

  // --- Expanded panel ---
  return (
    <div className="shrink-0 w-[280px] bg-panel border-l border-default flex flex-col overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-base py-half border-b border-default">
        <span className="text-sm font-medium text-high">
          {t('conversation.planning.auditDoc.panelTitle')}
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
            className="size-3"
          >
            <path d="M5.28 4.22a.75.75 0 0 0-1.06 1.06L6.94 8l-2.72 2.72a.75.75 0 1 0 1.06 1.06L8 9.06l2.72 2.72a.75.75 0 1 0 1.06-1.06L9.06 8l2.72-2.72a.75.75 0 0 0-1.06-1.06L8 6.94 5.28 4.22Z" />
          </svg>
        </button>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-base space-y-base">
        {/* Upload area */}
        {!hasDoc && (
          <div
            role="button"
            tabIndex={isReadOnly ? -1 : 0}
            className={`border-2 border-dashed rounded-lg p-double flex flex-col items-center justify-center gap-half text-center cursor-pointer transition-colors ${
              isReadOnly
                ? 'border-default opacity-50 cursor-not-allowed'
                : isDragOver
                  ? 'border-brand bg-brand/5'
                  : 'border-default hover:border-brand/50'
            }`}
            onClick={() => !isReadOnly && fileInputRef.current?.click()}
            onKeyDown={(e) => {
              if (!isReadOnly && (e.key === 'Enter' || e.key === ' ')) {
                e.preventDefault();
                fileInputRef.current?.click();
              }
            }}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
          >
            {/* Upload icon */}
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 16 16"
              fill="currentColor"
              className="size-4 text-low"
            >
              <path
                fillRule="evenodd"
                d="M8 1a.75.75 0 0 1 .75.75v6.5a.75.75 0 0 1-1.5 0v-6.5A.75.75 0 0 1 8 1ZM4.11 4.972a.75.75 0 0 1 0 1.06L8 1.94l3.89 4.092a.75.75 0 0 1-1.06 1.06L8 4.06 5.17 6.032a.75.75 0 0 1-1.06-1.06Z"
                clipRule="evenodd"
              />
              <path d="M2 10a.75.75 0 0 1 .75.75v1.5c0 .138.112.25.25.25h10a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 13 14H3a1.75 1.75 0 0 1-1.75-1.75v-1.5A.75.75 0 0 1 2 10Z" />
            </svg>
            <span className="text-xs text-normal">
              {t('conversation.planning.auditDoc.uploadHint')}
            </span>
            <span className="text-xs text-low">
              {t('conversation.planning.auditDoc.uploadFormats')}
            </span>
            {uploadMutation.isPending && (
              <span className="text-xs text-brand animate-pulse">...</span>
            )}
          </div>
        )}

        {/* Hidden file input */}
        <input
          ref={fileInputRef}
          type="file"
          accept={ACCEPTED_EXTENSIONS}
          className="hidden"
          onChange={handleInputChange}
        />

        {/* Uploaded file display */}
        {hasDoc && (
          <div className="flex items-center gap-half bg-secondary rounded px-base py-half">
            {/* File icon */}
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 16 16"
              fill="currentColor"
              className="size-3 text-brand shrink-0"
            >
              <path d="M4 1.75A.75.75 0 0 1 4.75 1h4.914a.75.75 0 0 1 .53.22l3.586 3.586a.75.75 0 0 1 .22.53v8.914a.75.75 0 0 1-.75.75H4.75a.75.75 0 0 1-.75-.75V1.75Zm5.25 1.5v2.5a.75.75 0 0 0 .75.75h2.5L9.25 3.25Z" />
            </svg>
            <span className="text-xs text-normal truncate flex-1">
              {fileName}
            </span>
            {!isReadOnly && (
              <button
                type="button"
                onClick={handleRemove}
                disabled={deleteMutation.isPending}
                className="text-xs text-low hover:text-high transition-colors shrink-0 disabled:opacity-50"
              >
                {t('conversation.planning.auditDoc.remove')}
              </button>
            )}
          </div>
        )}

        {/* Read-only mode badge */}
        {isReadOnly && hasDoc && (
          <div className="flex items-center gap-half">
            <span className="text-xs px-half py-px rounded bg-brand/10 text-brand">
              {auditMode === 'merged'
                ? t('conversation.planning.auditDoc.retainBuiltin')
                : auditMode === 'custom'
                  ? t('conversation.planning.auditDoc.noRetainBuiltinDesc')
                  : ''}
            </span>
          </div>
        )}

        {/* Retain built-in toggle (only when doc is uploaded and not read-only) */}
        {hasDoc && !isReadOnly && (
          <div className="space-y-half">
            <label className="flex items-start gap-half cursor-pointer">
              <input
                type="checkbox"
                checked={retainBuiltin}
                onChange={(e) => onRetainBuiltinChange(e.target.checked)}
                className="mt-px shrink-0 accent-brand"
              />
              <span className="text-xs text-normal">
                {t('conversation.planning.auditDoc.retainBuiltin')}
              </span>
            </label>
            <p className="text-xs text-low pl-4">
              {retainBuiltin
                ? t('conversation.planning.auditDoc.retainBuiltinDesc')
                : t('conversation.planning.auditDoc.noRetainBuiltinDesc')}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
