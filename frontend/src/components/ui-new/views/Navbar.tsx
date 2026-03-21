import type { Icon } from '@phosphor-icons/react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { Tooltip } from '../primitives/Tooltip';
import {
  type ActionDefinition,
  type ActionVisibilityContext,
  type NavbarItem,
  isSpecialIcon,
} from '../actions';
import {
  isActionActive,
  isActionEnabled,
  getActionIcon,
  getActionTooltip,
} from '../actions/useActionVisibility';

/**
 * Check if a NavbarItem is a divider
 */
function isDivider(item: NavbarItem): item is { readonly type: 'divider' } {
  return 'type' in item && item.type === 'divider';
}

/**
 * Mapping from English tooltip strings to i18n keys under common:navbarActions.
 * Covers both static labels and dynamic tooltip variants for all navbar actions.
 */
const TOOLTIP_I18N_MAP: Record<string, string> = {
  // Archive action
  'Archive': 'archive-workspace',
  'Unarchive': 'unarchive-workspace',
  // Old UI
  'Open in Old UI': 'open-in-old-ui',
  // Diff view mode
  'Inline view': 'toggle-diff-view-mode-inline',
  'Side-by-side view': 'toggle-diff-view-mode-split',
  // All diffs
  'Collapse all diffs': 'toggle-all-diffs-collapse',
  'Expand all diffs': 'toggle-all-diffs-expand',
  // Left sidebar
  'Hide Left Sidebar': 'toggle-left-sidebar-hide',
  'Show Left Sidebar': 'toggle-left-sidebar-show',
  // Chat panel
  'Toggle Chat Panel': 'toggle-left-main-panel',
  'Hide Chat Panel': 'hide-chat-panel',
  'Show Chat Panel': 'show-chat-panel',
  // Changes panel
  'Toggle Changes Panel': 'toggle-changes-mode',
  'Hide Changes Panel': 'hide-changes-panel',
  'Show Changes Panel': 'show-changes-panel',
  // Logs panel
  'Toggle Logs Panel': 'toggle-logs-mode',
  'Hide Logs Panel': 'hide-logs-panel',
  'Show Logs Panel': 'show-logs-panel',
  // Preview panel
  'Toggle Preview Panel': 'toggle-preview-mode',
  'Hide Preview Panel': 'hide-preview-panel',
  'Show Preview Panel': 'show-preview-panel',
  // Right sidebar
  'Hide Right Sidebar': 'toggle-right-sidebar-hide',
  'Show Right Sidebar': 'toggle-right-sidebar-show',
  // Global actions
  'New Workspace': 'new-workspace',
  'Open Command Bar': 'open-command-bar',
  'Give Feedback': 'feedback',
  'Workspaces Guide': 'workspaces-guide',
  'Settings': 'settings',
  // Review
  'Ask the agent to review your changes': 'start-review',
};

// NavbarIconButton - inlined from primitives
interface NavbarIconButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  icon: Icon;
  isActive?: boolean;
  tooltip?: string;
}

function NavbarIconButton({
  icon: IconComponent,
  isActive = false,
  tooltip,
  className,
  ...props
}: Readonly<NavbarIconButtonProps>) {
  const button = (
    <button
      type="button"
      className={cn(
        'flex items-center justify-center rounded-sm',
        'text-low hover:text-normal',
        isActive && 'text-normal',
        className
      )}
      {...props}
    >
      <IconComponent
        className="size-icon-base"
        weight={isActive ? 'fill' : 'regular'}
      />
    </button>
  );

  return tooltip ? <Tooltip content={tooltip}>{button}</Tooltip> : button;
}

export interface NavbarProps {
  workspaceTitle?: string;
  // Items for left side of navbar
  leftItems?: NavbarItem[];
  // Items for right side of navbar (with dividers inline)
  rightItems?: NavbarItem[];
  // Context for deriving action state
  actionContext: ActionVisibilityContext;
  // Handler to execute an action
  onExecuteAction: (action: ActionDefinition) => void;
  className?: string;
}

export function Navbar({
  workspaceTitle = 'Workspace Title',
  leftItems = [],
  rightItems = [],
  actionContext,
  onExecuteAction,
  className,
}: Readonly<NavbarProps>) {
  const { t } = useTranslation('common');

  const translateTooltip = (tooltipText: string): string => {
    const i18nKey = TOOLTIP_I18N_MAP[tooltipText];
    if (i18nKey) {
      return t(`navbarActions.${i18nKey}`, tooltipText);
    }
    return tooltipText;
  };

  const renderItem = (item: NavbarItem, key: string) => {
    // Render divider
    if (isDivider(item)) {
      return <div key={key} className="h-4 w-px bg-border" />;
    }

    // Render action - derive state from action callbacks
    const action = item;
    const active = isActionActive(action, actionContext);
    const enabled = isActionEnabled(action, actionContext);
    const iconOrSpecial = getActionIcon(action, actionContext);
    const tooltip = translateTooltip(getActionTooltip(action, actionContext));
    const isDisabled = !enabled;

    // Skip special icons in navbar (navbar only uses standard phosphor icons)
    if (isSpecialIcon(iconOrSpecial)) {
      return null;
    }

    return (
      <NavbarIconButton
        key={key}
        icon={iconOrSpecial}
        isActive={active}
        onClick={() => onExecuteAction(action)}
        aria-label={tooltip}
        tooltip={tooltip}
        disabled={isDisabled}
        className={isDisabled ? 'opacity-40 cursor-not-allowed' : ''}
      />
    );
  };

  return (
    <nav
      className={cn(
        'flex items-center justify-between px-base py-half bg-secondary border-b shrink-0',
        className
      )}
    >
      {/* Left - Archive & Old UI Link */}
      <div className="flex-1 flex items-center gap-base">
        {leftItems.map((item, index) =>
          renderItem(
            item,
            `left-${isDivider(item) ? 'divider' : item.id}-${index}`
          )
        )}
      </div>

      {/* Center - Workspace Title */}
      <div className="flex-1 flex items-center justify-center">
        <p className="text-base text-low truncate">{workspaceTitle}</p>
      </div>

      {/* Right - Diff Controls + Panel Toggles (dividers inline) */}
      <div className="flex-1 flex items-center justify-end gap-base">
        {rightItems.map((item, index) =>
          renderItem(
            item,
            `right-${isDivider(item) ? 'divider' : item.id}-${index}`
          )
        )}
      </div>
    </nav>
  );
}
