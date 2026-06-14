import { type Icon } from '@phosphor-icons/react';
import { cn } from '@/lib/utils';
import {
  DropdownMenu,
  DropdownMenuTriggerButton,
  DropdownMenuContent,
} from './Dropdown';

interface ToolbarProps extends React.HTMLAttributes<HTMLDivElement> {
  readonly children: React.ReactNode;
}

function Toolbar({ children, className, ...props }: Readonly<ToolbarProps>) {
  return (
    <div className={cn('flex items-center gap-base', className)} {...props}>
      {children}
    </div>
  );
}

interface ToolbarIconButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  readonly icon: Icon;
}

function ToolbarIconButton({
  icon: IconComponent,
  className,
  disabled,
  ...props
}: Readonly<ToolbarIconButtonProps>) {
  return (
    <button
      className={cn(
        'flex items-center justify-center text-low hover:text-normal',
        disabled && 'opacity-40 cursor-not-allowed hover:text-low',
        className
      )}
      disabled={disabled}
      {...props}
    >
      <IconComponent className="size-icon-base" />
    </button>
  );
}

interface ToolbarDropdownProps {
  readonly label: string;
  readonly icon?: Icon;
  readonly children?: React.ReactNode;
  readonly className?: string;
  readonly disabled?: boolean;
}

function ToolbarDropdown({
  label,
  icon,
  children,
  className,
  disabled,
}: Readonly<ToolbarDropdownProps>) {
  return (
    <DropdownMenu>
      <DropdownMenuTriggerButton
        icon={icon}
        label={label}
        className={className}
        disabled={disabled}
      />
      <DropdownMenuContent>
        {children}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export { Toolbar, ToolbarIconButton, ToolbarDropdown };
