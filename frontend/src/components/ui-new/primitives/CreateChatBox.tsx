import { useRef } from 'react';
import { CheckIcon, PaperclipIcon } from '@phosphor-icons/react';
import { useTranslation } from 'react-i18next';
import { toPrettyCase } from '@/utils/string';
import type { BaseCodingAgent } from 'shared/types';
import type { ModelOption } from '@/hooks/useModelConfigForExecutor';
import type { LocalImageMetadata } from '@/components/ui/wysiwyg/context/task-attempt-context';
import { AgentIcon } from '@/components/agents/AgentIcon';
import { Checkbox } from '@/components/ui/checkbox';
import {
  ChatBoxBase,
  VisualVariant,
  type EditorProps,
  type VariantProps,
} from './ChatBoxBase';
import { PrimaryButton } from './PrimaryButton';
import { ToolbarDropdown, ToolbarIconButton } from './Toolbar';
import {
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from './Dropdown';

export interface ExecutorProps {
  selected: BaseCodingAgent | null;
  options: BaseCodingAgent[];
  onChange: (executor: BaseCodingAgent) => void;
}

export interface ModelConfigProps {
  customModels: ModelOption[];
  officialModels: ModelOption[];
  selectedId: string | null;
  onChange: (id: string | null) => void;
}

export interface SaveAsDefaultProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  visible: boolean;
}

interface CreateChatBoxProps {
  readonly editor: EditorProps;
  readonly onSend: () => void;
  readonly isSending: boolean;
  readonly executor: ExecutorProps;
  readonly modelConfig?: ModelConfigProps;
  readonly variant?: VariantProps;
  readonly saveAsDefault?: SaveAsDefaultProps;
  readonly error?: string | null;
  readonly projectId?: string;
  readonly agent?: BaseCodingAgent | null;
  readonly onPasteFiles?: (files: File[]) => void;
  /** Local images for immediate preview (before saved to server) */
  readonly localImages?: LocalImageMetadata[];
}

/**
 * Lightweight chat box for create mode.
 * Supports sending and attachments - no queue, stop, or feedback functionality.
 */
export function CreateChatBox({
  editor,
  onSend,
  isSending,
  executor,
  modelConfig,
  variant,
  saveAsDefault,
  error,
  projectId,
  agent,
  onPasteFiles,
  localImages,
}: Readonly<CreateChatBoxProps>) {
  const { t } = useTranslation('tasks');
  const fileInputRef = useRef<HTMLInputElement>(null);
  const canSend = editor.value.trim().length > 0 && !isSending;

  const handleCmdEnter = () => {
    if (canSend) {
      onSend();
    }
  };

  const handleAttachClick = () => {
    fileInputRef.current?.click();
  };

  const handleFileInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || []).filter((f) =>
      f.type.startsWith('image/')
    );
    if (files.length > 0 && onPasteFiles) {
      onPasteFiles(files);
    }
    e.target.value = '';
  };

  const executorLabel = executor.selected
    ? toPrettyCase(executor.selected)
    : 'Select Executor';

  return (
    <ChatBoxBase
      editor={editor}
      placeholder="Describe the task..."
      onCmdEnter={handleCmdEnter}
      disabled={isSending}
      projectId={projectId}
      autoFocus
      variant={variant}
      error={error}
      visualVariant={VisualVariant.NORMAL}
      onPasteFiles={onPasteFiles}
      localImages={localImages}
      headerLeft={
        <>
          <AgentIcon agent={agent} className="size-icon-xl" />
          <ToolbarDropdown label={executorLabel}>
            <DropdownMenuLabel>{t('conversation.executors')}</DropdownMenuLabel>
            {executor.options.map((exec) => (
              <DropdownMenuItem
                key={exec}
                icon={executor.selected === exec ? CheckIcon : undefined}
                onClick={() => executor.onChange(exec)}
              >
                {toPrettyCase(exec)}
              </DropdownMenuItem>
            ))}
          </ToolbarDropdown>
          {modelConfig && (
            <ToolbarDropdown
              label={
                [...modelConfig.customModels, ...modelConfig.officialModels]
                  .find((m) => m.id === modelConfig.selectedId)
                  ?.displayName ?? t('conversation.selectModel')
              }
            >
              {modelConfig.customModels.length > 0 && (
                <>
                  <DropdownMenuLabel>
                    {t('conversation.customModels')}
                  </DropdownMenuLabel>
                  {modelConfig.customModels.map((model) => (
                    <DropdownMenuItem
                      key={model.id}
                      icon={
                        modelConfig.selectedId === model.id
                          ? CheckIcon
                          : undefined
                      }
                      onClick={() => modelConfig.onChange(model.id)}
                    >
                      <span className="flex flex-col">
                        <span>{model.displayName}</span>
                        {model.subtitle && (
                          <span className="text-low text-xs">
                            {model.subtitle}
                          </span>
                        )}
                      </span>
                    </DropdownMenuItem>
                  ))}
                </>
              )}
              {modelConfig.officialModels.length > 0 && (
                <>
                  {modelConfig.customModels.length > 0 && (
                    <DropdownMenuSeparator />
                  )}
                  <DropdownMenuLabel>
                    {t('conversation.officialModels')}
                  </DropdownMenuLabel>
                  {modelConfig.officialModels.map((model) => (
                    <DropdownMenuItem
                      key={model.id}
                      icon={
                        modelConfig.selectedId === model.id
                          ? CheckIcon
                          : undefined
                      }
                      onClick={() => modelConfig.onChange(model.id)}
                    >
                      {model.displayName}
                    </DropdownMenuItem>
                  ))}
                </>
              )}
            </ToolbarDropdown>
          )}
          {saveAsDefault?.visible && (
            <label className="flex items-center gap-1.5 text-sm text-low cursor-pointer ml-2">
              <Checkbox
                checked={saveAsDefault.checked}
                onCheckedChange={saveAsDefault.onChange}
                className="h-3.5 w-3.5"
              />
              <span>{t('conversation.saveAsDefault')}</span>
            </label>
          )}
        </>
      }
      footerLeft={
        <>
          <ToolbarIconButton
            icon={PaperclipIcon}
            aria-label="Attach file"
            onClick={handleAttachClick}
            disabled={isSending}
          />
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            multiple
            className="hidden"
            onChange={handleFileInputChange}
          />
        </>
      }
      footerRight={
        <PrimaryButton
          onClick={onSend}
          disabled={!canSend}
          actionIcon={isSending ? 'spinner' : undefined}
          value={
            isSending
              ? t('conversation.workspace.creating')
              : t('conversation.workspace.create')
          }
        />
      }
    />
  );
}
