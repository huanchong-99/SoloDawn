import * as React from 'react';
import { cn } from '@/lib/utils';

interface KanbanCardProps extends React.HTMLAttributes<HTMLDivElement> {
  children: React.ReactNode;
  id?: string;
  name?: string;
  index?: number;
  parent?: string;
  isOpen?: boolean;
  forwardedRef?: React.RefObject<HTMLDivElement>;
  dragDisabled?: boolean;
}

export function KanbanCard({
  children,
  className,
  id: _id,
  name: _name,
  index: _index,
  parent: _parent,
  isOpen: _isOpen,
  forwardedRef,
  dragDisabled: _dragDisabled,
  ...props
}: Readonly<KanbanCardProps>) {
  // These destructured props are consumed by the drag-and-drop library at a higher level
  // and must be extracted here to avoid passing them to the DOM element via ...props.
  void _id; void _name; void _index; void _parent; void _isOpen; void _dragDisabled;
  return (
    <div
      ref={forwardedRef}
      className={cn(
        'rounded-lg border bg-card p-3 shadow-sm transition-colors hover:bg-accent/50',
        className
      )}
      {...props}
    >
      {children}
    </div>
  );
}
