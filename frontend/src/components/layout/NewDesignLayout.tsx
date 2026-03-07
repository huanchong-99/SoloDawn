import {
  Outlet,
  useLocation,
  useNavigate,
  useParams,
  useSearchParams,
} from 'react-router-dom';
import { LayoutGrid, GitBranch, Bug, Settings } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { LanguageToggleButton } from './LanguageToggleButton';
import { ProductModeSwitch } from './ProductModeSwitch';

type ViewType = 'kanban' | 'pipeline' | 'debug';

interface ViewOption {
  id: ViewType;
  labelKey: string;
  icon: React.ComponentType<{ className?: string }>;
  path: (workflowId?: string) => string;
  requiresWorkflow: boolean;
}

/**
 * View options matching actual route structure:
 * - /board (kanban, no workflow required)
 * - /pipeline/:workflowId (requires workflow)
 * - /debug/:workflowId (requires workflow)
 */
const VIEW_OPTIONS: ViewOption[] = [
  {
    id: 'kanban',
    labelKey: 'viewSwitcher.kanban',
    icon: LayoutGrid,
    path: (workflowId) =>
      workflowId ? `/board?workflowId=${encodeURIComponent(workflowId)}` : '/board',
    requiresWorkflow: false,
  },
  {
    id: 'pipeline',
    labelKey: 'viewSwitcher.pipeline',
    icon: GitBranch,
    path: (workflowId) =>
      workflowId ? `/pipeline/${encodeURIComponent(workflowId)}` : '/board',
    requiresWorkflow: true,
  },
  {
    id: 'debug',
    labelKey: 'viewSwitcher.debug',
    icon: Bug,
    path: (workflowId) =>
      workflowId ? `/debug/${encodeURIComponent(workflowId)}` : '/board',
    requiresWorkflow: true,
  },
];

/**
 * Determine current view from pathname
 */
function getCurrentView(pathname: string): ViewType {
  if (pathname.startsWith('/debug/')) return 'debug';
  if (pathname.startsWith('/pipeline/')) return 'pipeline';
  return 'kanban';
}

/**
 * Get button className based on state
 */
function getButtonClassName(isActive: boolean, isDisabled: boolean): string {
  if (isActive) return 'bg-brand/10 text-brand font-medium';
  if (isDisabled) return 'text-low/50 cursor-not-allowed';
  return 'text-low hover:text-high hover:bg-secondary';
}

export function NewDesignLayout() {
  const { t } = useTranslation('workflow');
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { workflowId: routeWorkflowId } = useParams<{ workflowId?: string }>();
  const boardWorkflowId = searchParams.get('workflowId') ?? undefined;
  const workflowId = routeWorkflowId ?? boardWorkflowId;

  const currentView = getCurrentView(location.pathname);

  const handleViewChange = (view: ViewOption) => {
    // Don't navigate to views that require workflow if we don't have one
    if (view.requiresWorkflow && !workflowId) {
      return;
    }
    navigate(view.path(workflowId));
  };

  const showViewSwitcher =
    location.pathname === '/board' ||
    location.pathname.startsWith('/pipeline/') ||
    location.pathname.startsWith('/debug/') ||
    location.pathname.startsWith('/workspaces') ||
    location.pathname.startsWith('/wizard') ||
    location.pathname.startsWith('/workflows');

  return (
    <div className="flex-1 min-h-0 overflow-hidden flex flex-col">
      {/* View Switcher Navigation */}
      {showViewSwitcher && (
        <div className="h-10 bg-panel border-b border-border px-4 flex items-center gap-1">
          <div className="flex items-center gap-1">
            {VIEW_OPTIONS.map((option) => {
              const Icon = option.icon;
              const isActive = currentView === option.id;
              const isDisabled = option.requiresWorkflow && !workflowId;
              const tooltipTitle = isDisabled
                ? t('viewSwitcher.selectWorkflowFirst')
                : undefined;
              return (
                <button
                  key={option.id}
                  onClick={() => handleViewChange(option)}
                  disabled={isDisabled}
                  className={cn(
                    'flex items-center gap-1.5 px-3 py-1.5 rounded text-sm transition-colors',
                    getButtonClassName(isActive, isDisabled)
                  )}
                  title={tooltipTitle}
                >
                  <Icon className="w-4 h-4" />
                  {t(option.labelKey)}
                </button>
              );
            })}
          </div>
          <div className="ml-auto flex items-center gap-1.5">
            <ProductModeSwitch />
            <div className="w-px h-4 bg-border" />
            <button
              type="button"
              onClick={() => navigate('/wizard')}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded text-sm text-low hover:text-high hover:bg-secondary"
              title={t('viewSwitcher.workflowManagement')}
            >
              {t('viewSwitcher.workflowManagement')}
            </button>
            <LanguageToggleButton className="h-auto min-w-0 px-3 py-1.5 rounded text-sm" />
            <button
              type="button"
              onClick={() => navigate('/settings')}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded text-sm text-low hover:text-high hover:bg-secondary"
              aria-label={t('viewSwitcher.settings')}
              title={t('viewSwitcher.settings')}
            >
              <Settings className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}

      {/* Main Content */}
      <div className="flex-1 min-h-0 overflow-hidden">
        <Outlet />
      </div>
    </div>
  );
}
