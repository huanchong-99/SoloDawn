import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import WYSIWYGEditor from '@/components/ui/wysiwyg';
import {
  ActionType,
  NormalizedEntry,
  ToolStatus,
  type NormalizedEntryType,
  type TaskWithAttemptStatus,
  type JsonValue,
} from 'shared/types.ts';
import type { WorkspaceWithSession } from '@/types/attempt';
import type { ProcessStartPayload } from '@/types/logs';
import FileChangeRenderer from './FileChangeRenderer';
import { useExpandable } from '@/stores/useExpandableStore';
import {
  WarningCircle as AlertCircle,
  Robot as Bot,
  Brain,
  CheckSquare,
  CaretDown as ChevronDown,
  Hammer,
  PencilSimple as Edit,
  Eye,
  Globe,
  Plus,
  MagnifyingGlass as Search,
  Gear as Settings,
  Terminal,
  User,
  Wrench,
} from '@phosphor-icons/react';
import RawLogText from '../common/RawLogText';
import UserMessage from './UserMessage';
import PendingApprovalEntry from './PendingApprovalEntry';
import { NextActionCard } from './NextActionCard';
import { cn } from '@/lib/utils';
import { useRetryUi } from '@/contexts/RetryUiContext';
import { Button } from '@/components/ui/button';
import {
  ScriptFixerDialog,
  type ScriptType,
} from '@/components/dialogs/scripts/ScriptFixerDialog';
import { useAttemptRepo } from '@/hooks/useAttemptRepo';

type Props = Readonly<{
  entry: NormalizedEntry | ProcessStartPayload;
  expansionKey: string;
  executionProcessId?: string;
  taskAttempt?: WorkspaceWithSession;
  task?: TaskWithAttemptStatus;
}>;

type FileEditAction = Extract<ActionType, { action: 'file_edit' }>;
type GenericToolAction = Extract<ActionType, { action: 'tool' }>;

const isNormalizedConversationEntry = (
  entry: NormalizedEntry | ProcessStartPayload
): entry is NormalizedEntry => 'entry_type' in entry;

const isProcessStartEntry = (
  entry: NormalizedEntry | ProcessStartPayload
): entry is ProcessStartPayload => 'processId' in entry;

const renderJson = (v: JsonValue) => (
  <pre className="whitespace-pre-wrap">{JSON.stringify(v, null, 2)}</pre>
);

// Helper function to check if it's a task management tool
const isTaskManagementTool = (action: string, toolName?: string): boolean => {
  if (action === 'todo_management') return true;
  if (!toolName) return false;
  const lowerName = toolName.toLowerCase();
  return ['todowrite', 'todoread', 'todo_write', 'todo_read', 'todo'].includes(
    lowerName
  );
};

const getEntryIcon = (entryType: NormalizedEntryType) => {
  const iconSize = 'h-3 w-3';

  // Simple type mappings
  const typeIconMap: Record<string, JSX.Element> = {
    user_message: <User className={iconSize} />,
    user_feedback: <User className={iconSize} />,
    assistant_message: <Bot className={iconSize} />,
    system_message: <Settings className={iconSize} />,
    thinking: <Brain className={iconSize} />,
    error_message: <AlertCircle className={iconSize} />,
  };

  if (entryType.type in typeIconMap) {
    return typeIconMap[entryType.type];
  }

  if (entryType.type === 'tool_use') {
    const { action_type, tool_name } = entryType;

    // Check for task management tools first
    if (isTaskManagementTool(action_type.action, tool_name)) {
      return <CheckSquare className={iconSize} />;
    }

    // Action type mappings
    const actionIconMap: Record<string, JSX.Element> = {
      file_read: <Eye className={iconSize} />,
      file_edit: <Edit className={iconSize} />,
      command_run: <Terminal className={iconSize} />,
      search: <Search className={iconSize} />,
      web_fetch: <Globe className={iconSize} />,
      task_create: <Plus className={iconSize} />,
      plan_presentation: <CheckSquare className={iconSize} />,
      tool: <Hammer className={iconSize} />,
    };

    return actionIconMap[action_type.action] || <Settings className={iconSize} />;
  }

  return <Settings className={iconSize} />;
};

type ExitStatusVisualisation = 'success' | 'error' | 'pending';

const getStatusIndicator = (entryType: NormalizedEntryType) => {
  let status_visualisation: ExitStatusVisualisation | null = null;
  if (
    entryType.type === 'tool_use' &&
    entryType.action_type.action === 'command_run'
  ) {
    status_visualisation = 'pending';
    if (entryType.action_type.result?.exit_status?.type === 'success') {
      if (entryType.action_type.result?.exit_status?.success) {
        status_visualisation = 'success';
      } else {
        status_visualisation = 'error';
      }
    } else if (
      entryType.action_type.result?.exit_status?.type === 'exit_code'
    ) {
      if (entryType.action_type.result?.exit_status?.code === 0) {
        status_visualisation = 'success';
      } else {
        status_visualisation = 'error';
      }
    }
  }

  // If pending, should be a pulsing primary-foreground
  const colorMap: Record<ExitStatusVisualisation, string> = {
    success: 'bg-green-300',
    error: 'bg-red-300',
    pending: 'bg-primary-foreground/50',
  };

  if (!status_visualisation) return null;

  return (
    <div className="relative">
      <div
        className={`${colorMap[status_visualisation]} h-1.5 w-1.5 rounded-full absolute -left-1 -bottom-4`}
      />
      {status_visualisation === 'pending' && (
        <div
          className={`${colorMap[status_visualisation]} h-1.5 w-1.5 rounded-full absolute -left-1 -bottom-4 animate-ping`}
        />
      )}
    </div>
  );
};

/**********************
 * Helper definitions *
 **********************/

const shouldRenderMarkdown = (entryType: NormalizedEntryType) =>
  entryType.type === 'assistant_message' ||
  entryType.type === 'system_message' ||
  entryType.type === 'thinking' ||
  entryType.type === 'tool_use';

const getContentClassName = (entryType: NormalizedEntryType) => {
  const base = ' whitespace-pre-wrap break-words';
  if (
    entryType.type === 'tool_use' &&
    entryType.action_type.action === 'command_run'
  )
    return `${base} font-mono`;

  // Keep content-only styling — no bg/padding/rounded here.
  if (entryType.type === 'error_message')
    return `${base} font-mono text-destructive`;

  if (entryType.type === 'thinking') return `${base} opacity-60`;

  if (
    entryType.type === 'tool_use' &&
    (entryType.action_type.action === 'todo_management' ||
      (entryType.tool_name &&
        ['todowrite', 'todoread', 'todo_write', 'todo_read', 'todo'].includes(
          entryType.tool_name.toLowerCase()
        )))
  )
    return `${base} font-mono text-zinc-800 dark:text-zinc-200`;

  if (
    entryType.type === 'tool_use' &&
    entryType.action_type.action === 'plan_presentation'
  )
    return `${base} text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-950/20 px-3 py-2 border-l-4 border-blue-400`;

  return base;
};

/*********************
 * Unified card      *
 *********************/

type CardVariant = 'system' | 'error';

const MessageCard: React.FC<{
  children: React.ReactNode;
  variant: CardVariant;
  expanded?: boolean;
  onToggle?: () => void;
}> = ({ children, variant, expanded, onToggle }) => {
  const frameBase =
    'border px-3 py-2 w-full bg-[hsl(var(--card))] border-[hsl(var(--border))]';
  const systemTheme = 'border-400/40 text-zinc-500';
  const errorTheme =
    'border-red-400/40 bg-red-50 dark:bg-[hsl(var(--card))] text-[hsl(var(--foreground))]';

  const content = (
    <div className="flex items-center gap-1.5">
      <div className="min-w-0 flex-1">{children}</div>
      {onToggle && (
        <ExpandChevron
          expanded={!!expanded}
          onClick={onToggle}
          variant={variant}
        />
      )}
    </div>
  );

  if (onToggle) {
    return (
      <button
        type="button"
        className={`${frameBase} ${
          variant === 'system' ? systemTheme : errorTheme
        } cursor-pointer text-left`}
        onClick={onToggle}
      >
        {content}
      </button>
    );
  }

  return (
    <div
      className={`${frameBase} ${
        variant === 'system' ? systemTheme : errorTheme
      }`}
    >
      {content}
    </div>
  );
};

/************************
 * Collapsible container *
 ************************/

type CollapsibleVariant = 'system' | 'error';

const ExpandChevron: React.FC<{
  expanded: boolean;
  onClick: () => void;
  variant: CollapsibleVariant;
}> = ({ expanded, onClick, variant }) => {
  const color =
    variant === 'system'
      ? 'text-700 dark:text-300'
      : 'text-red-700 dark:text-red-300';

  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className={`h-4 w-4 cursor-pointer transition-transform bg-transparent border-0 p-0 ${color} ${
        expanded ? '' : '-rotate-90'
      }`}
      aria-label={expanded ? 'Collapse' : 'Expand'}
    >
      <ChevronDown className="h-4 w-4" />
    </button>
  );
};

const CollapsibleEntry: React.FC<{
  content: string;
  markdown: boolean;
  expansionKey: string;
  variant: CollapsibleVariant;
  contentClassName: string;
  taskAttemptId?: string;
}> = ({
  content,
  markdown,
  expansionKey,
  variant,
  contentClassName,
  taskAttemptId,
}) => {
  const multiline = content.includes('\n');
  const [expanded, toggle] = useExpandable(`entry:${expansionKey}`, false);

  const Inner = (
    <div className={contentClassName}>
      {markdown ? (
        <WYSIWYGEditor
          value={content}
          disabled
          className="whitespace-pre-wrap break-words"
          taskAttemptId={taskAttemptId}
        />
      ) : (
        content
      )}
    </div>
  );

  const firstLine = content.split('\n')[0];
  const PreviewInner = (
    <div className={contentClassName}>
      {markdown ? (
        <WYSIWYGEditor
          value={firstLine}
          disabled
          className="whitespace-pre-wrap break-words"
          taskAttemptId={taskAttemptId}
        />
      ) : (
        firstLine
      )}
    </div>
  );

  if (!multiline) {
    return <MessageCard variant={variant}>{Inner}</MessageCard>;
  }

  return expanded ? (
    <MessageCard variant={variant} expanded={expanded} onToggle={toggle}>
      {Inner}
    </MessageCard>
  ) : (
    <MessageCard variant={variant} expanded={expanded} onToggle={toggle}>
      {PreviewInner}
    </MessageCard>
  );
};

type ToolStatusAppearance = 'default' | 'denied' | 'timed_out';

const PLAN_APPEARANCE: Record<
  ToolStatusAppearance,
  {
    border: string;
    headerBg: string;
    headerText: string;
    contentBg: string;
    contentText: string;
  }
> = {
  default: {
    border: 'border-blue-400/40',
    headerBg: 'bg-blue-50 dark:bg-blue-950/20',
    headerText: 'text-blue-700 dark:text-blue-300',
    contentBg: 'bg-blue-50 dark:bg-blue-950/20',
    contentText: 'text-blue-700 dark:text-blue-300',
  },
  denied: {
    border: 'border-red-400/40',
    headerBg: 'bg-red-50 dark:bg-red-950/20',
    headerText: 'text-red-700 dark:text-red-300',
    contentBg: 'bg-red-50 dark:bg-red-950/10',
    contentText: 'text-red-700 dark:text-red-300',
  },
  timed_out: {
    border: 'border-amber-400/40',
    headerBg: 'bg-amber-50 dark:bg-amber-950/20',
    headerText: 'text-amber-700 dark:text-amber-200',
    contentBg: 'bg-amber-50 dark:bg-amber-950/10',
    contentText: 'text-amber-700 dark:text-amber-200',
  },
};

const PlanPresentationCard: React.FC<{
  plan: string;
  expansionKey: string;
  defaultExpanded?: boolean;
  statusAppearance?: ToolStatusAppearance;
  taskAttemptId?: string;
}> = ({
  plan,
  expansionKey,
  defaultExpanded = false,
  statusAppearance = 'default',
  taskAttemptId,
}) => {
  const { t } = useTranslation('common');
  const [expanded, toggle] = useExpandable(
    `plan-entry:${expansionKey}`,
    defaultExpanded
  );
  const tone = PLAN_APPEARANCE[statusAppearance];

  return (
    <div className="inline-block w-full">
      <div
        className={cn('border w-full overflow-hidden rounded-sm', tone.border)}
      >
        <button
          onClick={(e: React.MouseEvent) => {
            e.preventDefault();
            toggle();
          }}
          title={
            expanded
              ? t('conversation.planToggle.hide')
              : t('conversation.planToggle.show')
          }
          className={cn(
            'w-full px-2 py-1.5 flex items-center gap-1.5 text-left border-b',
            tone.headerBg,
            tone.headerText,
            tone.border
          )}
        >
          <span className=" min-w-0 truncate">
            <span className="font-semibold">{t('conversation.plan')}</span>
          </span>
          <div className="ml-auto flex items-center gap-2">
            <ExpandChevron
              expanded={expanded}
              onClick={toggle}
              variant={statusAppearance === 'denied' ? 'error' : 'system'}
            />
          </div>
        </button>

        {expanded && (
          <div className={cn('px-3 py-2', tone.contentBg)}>
            <div className={cn('text-sm', tone.contentText)}>
              <WYSIWYGEditor
                value={plan}
                disabled
                className="whitespace-pre-wrap break-words"
                taskAttemptId={taskAttemptId}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

// Helper to render command expanded details
const CommandDetails: React.FC<{
  argsText: string | null;
  output: string | null;
  linkifyUrls: boolean;
  argsLabel: string;
  outputLabel: string;
}> = ({ argsText, output, linkifyUrls, argsLabel, outputLabel }) => (
  <>
    {argsText && (
      <>
        <div className="font-normal uppercase bg-background border-b border-dashed px-2 py-1">
          {argsLabel}
        </div>
        <div className="px-2 py-1">{argsText}</div>
      </>
    )}
    {output && (
      <>
        <div className="font-normal uppercase bg-background border-y border-dashed px-2 py-1">
          {outputLabel}
        </div>
        <div className="px-2 py-1">
          <RawLogText content={output} linkifyUrls={linkifyUrls} />
        </div>
      </>
    )}
  </>
);

// Helper to render generic tool expanded details
const GenericToolDetails: React.FC<{
  actionType?: GenericToolAction;
  argsLabel: string;
  resultLabel: string;
  taskAttemptId?: string;
}> = ({ actionType, argsLabel, resultLabel, taskAttemptId }) => {
  if (!actionType) return null;
  return (
    <>
      <div className="font-normal uppercase bg-background border-b border-dashed px-2 py-1">
        {argsLabel}
      </div>
      <div className="px-2 py-1">
        {renderJson(actionType.arguments)}
      </div>
      <div className="font-normal uppercase bg-background border-y border-dashed px-2 py-1">
        {resultLabel}
      </div>
      <div className="px-2 py-1">
        {actionType.result?.type.type === 'markdown' &&
          actionType.result.value && (
            <WYSIWYGEditor
              value={actionType.result.value?.toString()}
              disabled
              taskAttemptId={taskAttemptId}
            />
          )}
        {actionType.result?.type.type === 'json' &&
          renderJson(actionType.result.value)}
      </div>
    </>
  );
};

const ToolCallHeaderContent: React.FC<{
  entryType?: Extract<NormalizedEntryType, { type: 'tool_use' }>;
  summaryText: string;
}> = ({ entryType, summaryText }) => (
  <span className=" min-w-0 flex items-center gap-1.5">
    <span>
      {entryType && getStatusIndicator(entryType)}
      {entryType && getEntryIcon(entryType)}
    </span>
    <span className="text-sm font-mono">{summaryText}</span>
  </span>
);

const ToolCallDetailsContent: React.FC<{
  isCommand: boolean;
  isTool: boolean;
  actionType?: ActionType;
  argsText: string | null;
  output: string | null;
  linkifyUrls: boolean;
  taskAttemptId?: string;
  argsLabel: string;
  outputLabel: string;
  resultLabel: string;
}> = ({
  isCommand,
  isTool,
  actionType,
  argsText,
  output,
  linkifyUrls,
  taskAttemptId,
  argsLabel,
  outputLabel,
  resultLabel,
}) => {
  if (isCommand) {
    return (
      <CommandDetails
        argsText={argsText}
        output={output}
        linkifyUrls={linkifyUrls}
        argsLabel={argsLabel}
        outputLabel={outputLabel}
      />
    );
  }

  if (isTool) {
    return (
      <GenericToolDetails
        actionType={actionType as GenericToolAction}
        argsLabel={argsLabel}
        resultLabel={resultLabel}
        taskAttemptId={taskAttemptId}
      />
    );
  }

  return null;
};

const ToolCallCard: React.FC<{
  entry: NormalizedEntry | ProcessStartPayload;
  expansionKey: string;
  forceExpanded?: boolean;
  taskAttemptId?: string;
}> = ({ entry, expansionKey, forceExpanded = false, taskAttemptId }) => {
  const { t } = useTranslation('common');

  const isNormalizedEntry = isNormalizedConversationEntry(entry);
  const entryType =
    isNormalizedEntry && entry.entry_type.type === 'tool_use'
      ? entry.entry_type
      : undefined;

  const linkifyUrls = entryType?.tool_name === 'Tool Install Script';
  const [expanded, toggle] = useExpandable(
    `tool-entry:${expansionKey}`,
    linkifyUrls
  );
  const effectiveExpanded = forceExpanded || expanded;

  const actionType = entryType?.action_type;
  const isCommand = actionType?.action === 'command_run';
  const isTool = actionType?.action === 'tool';
  const label = isCommand ? 'Ran' : entryType?.tool_name || 'Tool';

  const inlineText = isNormalizedEntry ? entry.content.trim() : '';
  const showInlineSummary = inlineText !== '' && !/\r?\n/.test(inlineText);

  const commandResult = isCommand ? actionType.result : null;
  const output = commandResult?.output ?? null;
  let argsText: string | null = null;
  if (isCommand) {
    const fromArgs =
      typeof actionType.command === 'string' ? actionType.command : '';
    argsText = (fromArgs || inlineText).trim();
  }

  const hasExpandableDetails = isCommand
    ? Boolean(argsText) || Boolean(output)
    : (isTool && !!actionType.arguments) || (isTool && !!actionType.result);
  const summaryText = showInlineSummary ? inlineText : label;
  const detailTitle = effectiveExpanded
    ? t('conversation.toolDetailsToggle.hide')
    : t('conversation.toolDetailsToggle.show');

  return (
    <div className="inline-block w-full flex flex-col gap-4">
      {hasExpandableDetails ? (
        <button
          type="button"
          onClick={(e: React.MouseEvent) => {
            e.preventDefault();
            toggle();
          }}
          title={detailTitle}
          className="w-full flex items-center gap-1.5 text-left text-secondary-foreground"
        >
          <ToolCallHeaderContent entryType={entryType} summaryText={summaryText} />
        </button>
      ) : (
        <div className="w-full flex items-center gap-1.5 text-left text-secondary-foreground">
          <ToolCallHeaderContent entryType={entryType} summaryText={summaryText} />
        </div>
      )}

      {effectiveExpanded && (
        <div className="max-h-[200px] overflow-y-auto border">
          <ToolCallDetailsContent
            isCommand={isCommand}
            isTool={isTool}
            actionType={actionType}
            argsText={argsText}
            output={output}
            linkifyUrls={linkifyUrls}
            taskAttemptId={taskAttemptId}
            argsLabel={t('conversation.args')}
            outputLabel={t('conversation.output')}
            resultLabel={t('conversation.result')}
          />
        </div>
      )}
    </div>
  );
};

// Script tool names that can be fixed
const SCRIPT_TOOL_NAMES = new Set([
  'Setup Script',
  'Cleanup Script',
  'Tool Install Script',
]);

const getScriptType = (toolName: string): ScriptType => {
  if (toolName === 'Setup Script') return 'setup';
  if (toolName === 'Cleanup Script') return 'cleanup';
  return 'dev_server'; // Tool Install Script
};

const ScriptToolCallCard: React.FC<{
  entry: NormalizedEntry | ProcessStartPayload;
  expansionKey: string;
  taskAttemptId?: string;
  sessionId?: string;
  isFailed: boolean;
  toolName: string;
  forceExpanded?: boolean;
}> = ({
  entry,
  expansionKey,
  taskAttemptId,
  sessionId,
  isFailed,
  toolName,
  forceExpanded = false,
}) => {
  const { t } = useTranslation('common');
  const { repos } = useAttemptRepo(taskAttemptId);

  const handleFix = useCallback(() => {
    if (!taskAttemptId || repos.length === 0) return;

    const scriptType = getScriptType(toolName);

    ScriptFixerDialog.show({
      scriptType,
      repos,
      workspaceId: taskAttemptId,
      sessionId,
      initialRepoId: repos.length === 1 ? repos[0].id : undefined,
    });
  }, [toolName, taskAttemptId, sessionId, repos]);

  const canFix = taskAttemptId && repos.length > 0 && isFailed;

  return (
    <div className="flex items-start gap-2">
      <div className="flex-1">
        <ToolCallCard
          entry={entry}
          expansionKey={expansionKey}
          forceExpanded={forceExpanded}
          taskAttemptId={taskAttemptId}
        />
      </div>
      {canFix && (
        <Button
          variant="outline"
          size="sm"
          onClick={handleFix}
          className="shrink-0 gap-1"
        >
          <Wrench className="h-3 w-3" />
          {t('conversation.fixScript')}
        </Button>
      )}
    </div>
  );
};

const LoadingCard = () => {
  return (
    <div className="flex animate-pulse space-x-2 items-center">
      <div className="size-3 bg-foreground/10"></div>
      <div className="flex-1 h-3 bg-foreground/10"></div>
      <div className="flex-1 h-3"></div>
      <div className="flex-1 h-3"></div>
    </div>
  );
};

const isPendingApprovalStatus = (
  status: ToolStatus
): status is Extract<ToolStatus, { status: 'pending_approval' }> =>
  status.status === 'pending_approval';

const getToolStatusAppearance = (status: ToolStatus): ToolStatusAppearance => {
  if (status.status === 'denied') return 'denied';
  if (status.status === 'timed_out') return 'timed_out';
  return 'default';
};

// Helper to extract exit code from a command_run action
function getCommandExitCode(actionType: ActionType): number | null {
  if (actionType.action !== 'command_run') return null;
  return actionType.result?.exit_status?.type === 'exit_code'
    ? actionType.result.exit_status.code
    : null;
}

const isFileEdit = (a: ActionType): a is FileEditAction =>
  a.action === 'file_edit';

// Extracted from DisplayConversationEntry to reduce cognitive complexity
function renderToolUseBody(
  toolEntry: Extract<NormalizedEntryType, { type: 'tool_use' }>,
  isPendingApproval: boolean,
  defaultExpanded: boolean,
  statusAppearance: ToolStatusAppearance,
  entry: NormalizedEntry | ProcessStartPayload,
  expansionKey: string,
  taskAttempt?: WorkspaceWithSession,
) {
  if (isFileEdit(toolEntry.action_type)) {
    const fileEditAction = toolEntry.action_type;
    return (
      <div className="space-y-3">
        {fileEditAction.changes.map((change, idx) => (
          <FileChangeRenderer
            key={`${fileEditAction.path}-${idx}`}
            path={fileEditAction.path}
            change={change}
            expansionKey={`edit:${expansionKey}:${idx}`}
            defaultExpanded={defaultExpanded}
            statusAppearance={statusAppearance}
            forceExpanded={isPendingApproval}
          />
        ))}
      </div>
    );
  }

  if (toolEntry.action_type.action === 'plan_presentation') {
    return (
      <PlanPresentationCard
        plan={toolEntry.action_type.plan}
        expansionKey={expansionKey}
        defaultExpanded={defaultExpanded}
        statusAppearance={statusAppearance}
        taskAttemptId={taskAttempt?.id}
      />
    );
  }

  if (
    toolEntry.action_type.action === 'command_run' &&
    SCRIPT_TOOL_NAMES.has(toolEntry.tool_name)
  ) {
    const exitCode = getCommandExitCode(toolEntry.action_type);
    return (
      <ScriptToolCallCard
        entry={entry}
        expansionKey={expansionKey}
        taskAttemptId={taskAttempt?.id}
        sessionId={taskAttempt?.session?.id}
        isFailed={exitCode !== null && exitCode !== 0}
        toolName={toolEntry.tool_name}
        forceExpanded={isPendingApproval}
      />
    );
  }

  return (
    <ToolCallCard
      entry={entry}
      expansionKey={expansionKey}
      forceExpanded={isPendingApproval}
      taskAttemptId={taskAttempt?.id}
    />
  );
}

/*******************
 * Main component  *
 *******************/

export const DisplayConversationEntryMaxWidth = (props: Props) => {
  return <DisplayConversationEntry {...props} />;
};

function renderEntryBody(
  entry: NormalizedEntry,
  taskAttempt?: WorkspaceWithSession
) {
  const shouldUseMarkdown = shouldRenderMarkdown(entry.entry_type);
  const plainContent = entry.content;
  const contentBody = shouldUseMarkdown ? (
    <WYSIWYGEditor
      value={plainContent}
      disabled
      className="whitespace-pre-wrap break-words flex flex-col gap-1 font-light"
      taskAttemptId={taskAttempt?.id}
    />
  ) : (
    plainContent
  );

  return (
    <div className="px-4 py-2 text-sm">
      <div className={getContentClassName(entry.entry_type)}>{contentBody}</div>
    </div>
  );
}

function DisplayConversationEntry({
  entry,
  expansionKey,
  executionProcessId,
  taskAttempt,
  task,
}: Props) {
  const { t } = useTranslation('common');
  const { isProcessGreyed } = useRetryUi();
  const greyed = isProcessGreyed(executionProcessId);

  if (isProcessStartEntry(entry)) {
    return (
      <div className={greyed ? 'opacity-50 pointer-events-none' : undefined}>
        <ToolCallCard
          entry={entry}
          expansionKey={expansionKey}
          taskAttemptId={taskAttempt?.id}
        />
      </div>
    );
  }

  // Handle NormalizedEntry
  const entryType = entry.entry_type;
  switch (entryType.type) {
    case 'user_message':
      return (
        <UserMessage
          content={entry.content}
          executionProcessId={executionProcessId}
          taskAttempt={taskAttempt}
        />
      );
    case 'user_feedback':
      return (
        <div className="py-2">
          <div className="bg-background px-4 py-2 text-sm border-y border-dashed">
            <div
              className="text-xs mb-1 opacity-70"
              style={{ color: 'hsl(var(--destructive))' }}
            >
              {t('conversation.deniedByUser', {
                toolName: entryType.denied_tool,
              })}
            </div>
            <WYSIWYGEditor
              value={entry.content}
              disabled
              className="whitespace-pre-wrap break-words flex flex-col gap-1 font-light py-3"
              taskAttemptId={taskAttempt?.id}
            />
          </div>
        </div>
      );
    case 'tool_use': {
      const toolEntry = entryType;
      const status = toolEntry.status;
      const isPendingApproval = status.status === 'pending_approval';
      const defaultExpanded =
        isPendingApproval || toolEntry.action_type.action === 'plan_presentation';
      const body = renderToolUseBody(
        toolEntry,
        isPendingApproval,
        defaultExpanded,
        getToolStatusAppearance(status),
        entry,
        expansionKey,
        taskAttempt
      );
      const content = (
        <div
          className={`px-4 py-2 text-sm space-y-3 ${greyed ? 'opacity-50 pointer-events-none' : ''}`}
        >
          {body}
        </div>
      );

      if (isPendingApprovalStatus(status)) {
        return (
          <PendingApprovalEntry
            pendingStatus={status}
            executionProcessId={executionProcessId}
          >
            {content}
          </PendingApprovalEntry>
        );
      }

      return content;
    }
    case 'system_message':
    case 'error_message':
      return (
        <div
          className={`px-4 py-2 text-sm ${greyed ? 'opacity-50 pointer-events-none' : ''}`}
        >
          <CollapsibleEntry
            content={entry.content}
            markdown={shouldRenderMarkdown(entryType)}
            expansionKey={expansionKey}
            variant={entryType.type === 'system_message' ? 'system' : 'error'}
            contentClassName={getContentClassName(entryType)}
            taskAttemptId={taskAttempt?.id}
          />
        </div>
      );
    case 'loading':
      return (
        <div className="px-4 py-2 text-sm">
          <LoadingCard />
        </div>
      );
    case 'next_action':
      return (
        <div className="px-4 py-2 text-sm">
          <NextActionCard
            attemptId={taskAttempt?.id}
            sessionId={taskAttempt?.session?.id}
            containerRef={taskAttempt?.containerRef}
            failed={entryType.failed}
            execution_processes={entryType.execution_processes}
            task={task}
            needsSetup={entryType.needs_setup}
          />
        </div>
      );
    default:
      if (!isNormalizedConversationEntry(entry)) {
        return null;
      }
      return renderEntryBody(entry, taskAttempt);
  }
}

export default DisplayConversationEntryMaxWidth;
