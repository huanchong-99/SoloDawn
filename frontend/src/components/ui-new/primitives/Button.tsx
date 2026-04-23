import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '@/lib/utils';

// TODO (P3): `ring-offset-background` resolves via colors.background in
// tailwind.new.config.js (maps to --bg-primary). Valid; but other primitives use
// `ring-offset-surface-1`. Consider unifying across Button/Dialog/SettingsToggle
// (E06-01, E06-02, E06-08).
const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:opacity-60 disabled:cursor-not-allowed cursor-pointer',
  {
    variants: {
      variant: {
        primary: 'bg-brand text-on-brand hover:bg-brand-hover shadow-soft',
        secondary:
          'bg-surface-2 text-high border border-border hover:bg-surface-3',
        outline: 'border border-border text-high hover:bg-surface-2',
        ghost: 'text-high hover:bg-surface-2/70',
        destructive: 'bg-error text-on-brand hover:bg-error/90',
        glass:
          'bg-glass text-high border border-border/70 hover:bg-surface-1/70 backdrop-blur',
        link: 'text-brand hover:text-brand-hover underline-offset-4 hover:underline',
      },
      size: {
        xs: 'h-7 px-2 text-xs',
        sm: 'h-8 px-3 text-xs',
        md: 'h-9 px-4 text-sm',
        lg: 'h-11 px-5 text-base',
        icon: 'h-9 w-9 p-0',
      },
    },
    defaultVariants: {
      variant: 'primary',
      size: 'md',
    },
  }
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, type, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button';
    return (
      <Comp
        ref={ref}
        type={asChild ? undefined : type ?? 'button'}
        className={cn(buttonVariants({ variant, size }), className)}
        {...props}
      />
    );
  }
);
Button.displayName = 'Button';

export { Button, buttonVariants };
