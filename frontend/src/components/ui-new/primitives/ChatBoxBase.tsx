import { type ReactNode } from 'react';
import { CheckIcon } from '@phosphor-icons/react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { toPrettyCase } from '@/utils/string';
import WYSIWYGEditor from '@/components/ui/wysiwyg';
import type { LocalImageMetadata } from '@/components/ui/wysiwyg/context/task-attempt-context';
import { Toolbar, ToolbarDropdown } from './Toolbar';
import { DropdownMenuItem, DropdownMenuLabel } from './Dropdown';

function SendModeToggle({
  sendOnEnter,
  onToggle,
}: {
  readonly sendOnEnter: boolean;
  readonly onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="text-[10px] text-low hover:text-normal transition-colors whitespace-nowrap"
      title={sendOnEnter ? 'Enter sends, Shift+Enter for newline' : 'Ctrl+Enter sends, Enter for newline'}
    >
      {sendOnEnter ? '⏎ Send' : '⌃⏎ Send'}
    </button>
  );
}

export interface EditorProps {
  value: string;
  onChange: (value: string) => void;
}

export interface VariantProps {
  selected: string | null;
  options: string[];
  onChange: (variant: string | null) => void;
}

export enum VisualVariant {
  NORMAL = 'NORMAL',
  FEEDBACK = 'FEEDBACK',
  EDIT = 'EDIT',
  PLAN = 'PLAN',
}

interface ChatBoxBaseProps {
  // Editor
  readonly editor: EditorProps;
  readonly placeholder: string;
  readonly onCmdEnter: () => void;
  readonly disabled?: boolean;
  readonly workspaceId?: string;
  readonly projectId?: string;
  readonly autoFocus?: boolean;

  // Variant selection
  readonly variant?: VariantProps;

  // Error display
  readonly error?: string | null;

  // Header content (right side - session/executor dropdown)
  readonly headerRight?: ReactNode;

  // Header content (left side - stats)
  readonly headerLeft?: ReactNode;

  // Footer left content (additional toolbar items like attach button)
  readonly footerLeft?: ReactNode;

  // Footer right content (action buttons)
  readonly footerRight: ReactNode;

  // Banner content (queued message indicator, feedback mode indicator)
  readonly banner?: ReactNode;

  // visualVariant
  readonly visualVariant: VisualVariant;

  // File paste handler for attachments
  readonly onPasteFiles?: (files: File[]) => void;

  // Whether the workspace is running (shows animated border)
  readonly isRunning?: boolean;

  // Key to force editor remount (e.g., when entering feedback mode to trigger auto-focus)
  readonly focusKey?: string;

  // Local images for immediate preview (before saved to server)
  readonly localImages?: LocalImageMetadata[];

  // Send mode toggle
  readonly sendOnEnter?: boolean;
  readonly onToggleSendMode?: () => void;
}

/**
 * Base chat box layout component.
 * Provides shared structure for CreateChatBox and SessionChatBox.
 */
export function ChatBoxBase({
  editor,
  placeholder,
  onCmdEnter,
  disabled,
  workspaceId,
  projectId,
  autoFocus,
  variant,
  error,
  headerRight,
  headerLeft,
  footerLeft,
  footerRight,
  banner,
  visualVariant,
  onPasteFiles,
  isRunning,
  focusKey,
  localImages,
  sendOnEnter,
  onToggleSendMode,
}: Readonly<ChatBoxBaseProps>) {
  const { t } = useTranslation('common');
  const variantLabel = toPrettyCase(variant?.selected || 'DEFAULT');
  const variantOptions = variant?.options ?? [];

  return (
    <div
      className={cn(
        'flex w-chat max-w-full flex-col border-t',
        '@chat:border-x @chat:rounded-t-md',
        (visualVariant === VisualVariant.FEEDBACK ||
          visualVariant === VisualVariant.EDIT ||
          visualVariant === VisualVariant.PLAN) &&
          'border-brand bg-brand/10',
        isRunning && 'chat-box-running'
      )}
    >
      {/* Error alert */}
      {error && (
        <div className="bg-error/10 border-b px-double py-base">
          <p className="text-error text-sm">{error}</p>
        </div>
      )}

      {/* Banner content (queued indicator, feedback mode, etc.) */}
      {banner}

      {/* Header - Stats and selector */}
      {visualVariant === VisualVariant.NORMAL && (
        <div className="flex items-center gap-base bg-secondary px-base py-[9px] @chat:rounded-t-md border-b">
          <div className="flex flex-1 items-center gap-base text-sm">
            {headerLeft}
          </div>
          <Toolbar className="gap-[9px]">{headerRight}</Toolbar>
        </div>
      )}

      {/* Editor area */}
      <div className="flex flex-col gap-plusfifty px-base py-base rounded-md">
        <WYSIWYGEditor
          key={focusKey}
          placeholder={placeholder}
          value={editor.value}
          onChange={editor.onChange}
          onCmdEnter={onCmdEnter}
          disabled={disabled}
          className="min-h-0 max-h-[50vh] overflow-y-auto"
          workspaceId={workspaceId}
          projectId={projectId}
          autoFocus={autoFocus}
          onPasteFiles={onPasteFiles}
          localImages={localImages}
        />

        {/* Footer - Controls */}
        <div className="flex items-end justify-between">
          <Toolbar className="flex-1 gap-double">
            {(visualVariant === VisualVariant.NORMAL ||
              visualVariant === VisualVariant.EDIT) &&
              variant &&
              variantOptions.length > 0 && (
                <ToolbarDropdown label={variantLabel} disabled={disabled}>
                  <DropdownMenuLabel>{t('chatBox.variants')}</DropdownMenuLabel>
                  {variantOptions.map((variantName) => (
                    <DropdownMenuItem
                      key={variantName}
                      icon={
                        variant?.selected === variantName
                          ? CheckIcon
                          : undefined
                      }
                      onClick={() => variant?.onChange(variantName)}
                    >
                      {toPrettyCase(variantName)}
                    </DropdownMenuItem>
                  ))}
                </ToolbarDropdown>
              )}
            {footerLeft}
            {onToggleSendMode != null && sendOnEnter != null && (
              <SendModeToggle sendOnEnter={sendOnEnter} onToggle={onToggleSendMode} />
            )}
          </Toolbar>
          <div className="flex gap-base">{footerRight}</div>
        </div>
      </div>
    </div>
  );
}
