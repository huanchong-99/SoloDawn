import { cn } from '@/lib/utils';
import { ToolStatus } from 'shared/types';

interface ToolStatusDotProps {
  readonly status: ToolStatus;
  readonly className?: string;
}

export function ToolStatusDot({ status, className }: Readonly<ToolStatusDotProps>) {
  const statusType = status.status;

  // Map status to visual state
  const isSuccess = statusType === 'success';
  const isError =
    statusType === 'failed' ||
    statusType === 'denied' ||
    statusType === 'timed_out';
  const isPending =
    statusType === 'created' || statusType === 'pending_approval';

  return (
    <span className={cn('relative inline-flex', className)}>
      <span
        className={cn(
          'size-1.5 rounded-full',
          isSuccess && 'bg-success',
          isError && 'bg-error',
          isPending && 'bg-low'
        )}
      />
      {isPending && (
        <span className="absolute inset-0 size-1.5 rounded-full bg-low animate-ping" />
      )}
    </span>
  );
}
