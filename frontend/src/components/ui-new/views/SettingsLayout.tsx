import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';
import type { Icon } from '@phosphor-icons/react';
import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react';

export interface SettingsNavItem {
  path: string;
  label: string;
  icon: Icon;
  visible?: boolean;
}

export interface SettingsLayoutViewProps {
  navItems: SettingsNavItem[];
  currentPath: string;
  onNavigate: (path: string) => void;
  onRerunSetup: () => void;
  rerunSetupLabel: string;
  title: string;
  children: ReactNode;
}

export function SettingsLayout({
  navItems,
  currentPath,
  onNavigate,
  onRerunSetup,
  rerunSetupLabel,
  title,
  children,
}: Readonly<SettingsLayoutViewProps>) {
  const visibleItems = navItems.filter(
    (item) => item.visible === undefined || item.visible
  );

  return (
    <div className="flex h-full overflow-hidden">
      {/* Sidebar */}
      <aside className="hidden md:flex w-56 shrink-0 flex-col bg-secondary border-r overflow-y-auto">
        {/* Title */}
        <div className="px-base py-double">
          <h1 className="text-lg font-semibold text-high">{title}</h1>
        </div>

        {/* Navigation */}
        <nav className="flex-1 flex flex-col gap-px px-half">
          {visibleItems.map((item) => {
            const IconComponent = item.icon;
            const isActive = currentPath.endsWith(item.path);

            return (
              <button
                key={item.path}
                type="button"
                onClick={() => onNavigate(item.path)}
                className={cn(
                  'flex items-center gap-base px-base py-half rounded text-sm text-normal',
                  'hover:bg-panel hover:text-high transition-colors',
                  isActive && 'bg-panel text-high border-l-2 border-brand'
                )}
              >
                <IconComponent
                  className="size-icon-sm shrink-0"
                  weight={isActive ? 'fill' : 'regular'}
                />
                <span className="truncate">{item.label}</span>
              </button>
            );
          })}
        </nav>

        {/* Re-run Setup Wizard Button */}
        <div className="px-half py-base mt-auto border-t">
          <button
            type="button"
            onClick={onRerunSetup}
            className={cn(
              'flex items-center gap-half w-full px-base py-half rounded',
              'text-xs text-low hover:text-normal hover:bg-panel transition-colors'
            )}
          >
            <ArrowCounterClockwiseIcon className="size-icon-xs shrink-0" />
            <span className="truncate">{rerunSetupLabel}</span>
          </button>
        </div>
      </aside>

      {/* Mobile sidebar (shown as horizontal nav on small screens) */}
      <div className="flex flex-col flex-1 min-w-0">
        <nav className="flex md:hidden items-center gap-half px-base py-half bg-secondary border-b overflow-x-auto">
          {visibleItems.map((item) => {
            const IconComponent = item.icon;
            const isActive = currentPath.endsWith(item.path);

            return (
              <button
                key={item.path}
                type="button"
                onClick={() => onNavigate(item.path)}
                className={cn(
                  'flex items-center gap-half px-half py-half rounded text-xs text-normal whitespace-nowrap',
                  'hover:bg-panel hover:text-high transition-colors',
                  isActive && 'bg-panel text-high'
                )}
              >
                <IconComponent
                  className="size-icon-xs shrink-0"
                  weight={isActive ? 'fill' : 'regular'}
                />
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>

        {/* Content Area */}
        <main className="flex-1 overflow-y-auto p-double">{children}</main>
      </div>
    </div>
  );
}
