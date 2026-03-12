import type { RefObject } from 'react';
import { Virtuoso, VirtuosoHandle } from 'react-virtuoso';
import { cn } from '@/lib/utils';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSearchInput,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './Dropdown';

interface SearchableDropdownProps<T> {
  /** Array of filtered items to display */
  readonly filteredItems: T[];
  /** Currently selected value (matched against getItemKey) */
  readonly selectedValue?: string | null;

  /** Extract unique key from item */
  readonly getItemKey: (item: T) => string;
  /** Extract display label from item */
  readonly getItemLabel: (item: T) => string;

  /** Called when an item is selected */
  readonly onSelect: (item: T) => void;

  /** Trigger element (uses asChild pattern) */
  readonly trigger: React.ReactNode;

  /** Search state */
  readonly searchTerm: string;
  readonly onSearchTermChange: (value: string) => void;

  /** Highlight state */
  readonly highlightedIndex: number | null;
  readonly onHighlightedIndexChange: (index: number | null) => void;

  /** Open state */
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;

  /** Keyboard handler */
  readonly onKeyDown: (e: React.KeyboardEvent) => void;

  /** Virtuoso ref for scrolling */
  readonly virtuosoRef: RefObject<VirtuosoHandle | null>;

  /** Class name for dropdown content */
  readonly contentClassName?: string;
  /** Placeholder text for search input */
  readonly placeholder?: string;
  /** Message shown when no items match */
  readonly emptyMessage?: string;

  /** Optional badge text for each item */
  readonly getItemBadge?: (item: T) => string | undefined;
}

export function SearchableDropdown<T>({
  filteredItems,
  selectedValue,
  getItemKey,
  getItemLabel,
  onSelect,
  trigger,
  searchTerm,
  onSearchTermChange,
  highlightedIndex,
  onHighlightedIndexChange,
  open,
  onOpenChange,
  onKeyDown,
  virtuosoRef,
  contentClassName,
  placeholder = 'Search',
  emptyMessage = 'No items found',
  getItemBadge,
}: SearchableDropdownProps<T>) {
  const renderItem = (idx: number) => {
    const item = filteredItems[idx];
    const key = getItemKey(item);
    const isHighlighted = idx === highlightedIndex;
    const isSelected = selectedValue === key;
    return (
      <DropdownMenuItem
        onSelect={() => onSelect(item)}
        onMouseEnter={() => onHighlightedIndexChange(idx)}
        preventFocusOnHover
        badge={getItemBadge?.(item)}
        className={cn(
          isSelected && 'bg-secondary',
          isHighlighted && 'bg-secondary'
        )}
      >
        {getItemLabel(item)}
      </DropdownMenuItem>
    );
  };

  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent className={contentClassName}>
        <DropdownMenuSearchInput
          placeholder={placeholder}
          value={searchTerm}
          onValueChange={onSearchTermChange}
          onKeyDown={onKeyDown}
        />
        <DropdownMenuSeparator />
        {filteredItems.length === 0 ? (
          <div className="px-base py-half text-sm text-low text-center">
            {emptyMessage}
          </div>
        ) : (
          <Virtuoso
            ref={virtuosoRef as React.RefObject<VirtuosoHandle>}
            style={{ height: '16rem' }}
            totalCount={filteredItems.length}
            computeItemKey={(idx) =>
              getItemKey(filteredItems[idx]) ?? String(idx)
            }
            itemContent={renderItem}
          />
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
