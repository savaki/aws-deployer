import type {JSX} from 'solid-js'
import {splitProps} from 'solid-js'

type BadgeVariant = 'default' | 'success' | 'destructive' | 'warning' | 'secondary' | 'outline'

interface BadgeProps extends JSX.HTMLAttributes<HTMLSpanElement> {
    variant?: BadgeVariant
}

export function Badge(props: BadgeProps) {
    const [local, others] = splitProps(props, ['variant', 'class', 'children'])

    const variants = {
        default: 'bg-primary text-primary-foreground hover:bg-primary/80',
        success: 'bg-success text-success-foreground hover:bg-success/80',
        destructive: 'bg-destructive text-destructive-foreground hover:bg-destructive/80',
        warning: 'bg-warning text-warning-foreground hover:bg-warning/80',
        secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
        outline: 'border border-border text-foreground'
    }

    const variantClass = variants[local.variant || 'default']

    return (
        <span
            class={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 ${variantClass} ${local.class || ''}`}
            {...others}
        >
      {local.children}
    </span>
    )
}
