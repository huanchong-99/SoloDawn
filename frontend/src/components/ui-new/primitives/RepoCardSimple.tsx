import { XIcon, GitBranchIcon } from '@phosphor-icons/react';
import { cn } from '@/lib/utils';
import { SearchableDropdownContainer } from '@/components/ui-new/containers/SearchableDropdownContainer';
import { DropdownMenuTriggerButton } from '@/components/ui-new/primitives/Dropdown';
import type { GitBranch } from 'shared/types';

interface RepoCardSimpleProps {
  readonly name: string;
  readonly path: string;
  readonly onRemove?: () => void;
  readonly className?: string;
  readonly branches?: GitBranch[];
  readonly selectedBranch?: string | null;
  readonly onBranchChange?: (branch: string) => void;
}

export function RepoCardSimple({
  name,
  path,
  onRemove,
  className,
  branches,
  selectedBranch,
  onBranchChange,
}: Readonly<RepoCardSimpleProps>) {
  return (
    <div
      className={cn('flex flex-col gap-half bg-secondary rounded-sm', className)}
    >
      <div className="flex items-center gap-base text-normal ">
        <div className="flex-1 flex items-center gap-half">
          <p className="truncate">{name}</p>
        </div>
        {onRemove && (
          <button
            type="button"
            onClick={onRemove}
            className="text-low hover:text-normal flex-shrink-0"
          >
            <XIcon className="size-icon-xs" weight="bold" />
          </button>
        )}
      </div>
      <p className="text-xs text-low truncate">{path}</p>

      {branches && onBranchChange && (
        <SearchableDropdownContainer
          items={branches}
          selectedValue={selectedBranch}
          getItemKey={(b) => b.name}
          getItemLabel={(b) => b.name}
          getItemBadge={(b) => (b.is_current ? 'Current' : undefined)}
          onSelect={(b) => onBranchChange(b.name)}
          placeholder="Search"
          emptyMessage="No branches found"
          contentClassName="w-[280px]"
          trigger={
            <DropdownMenuTriggerButton
              icon={GitBranchIcon}
              label={selectedBranch || 'Select branch'}
            />
          }
        />
      )}
    </div>
  );
}
