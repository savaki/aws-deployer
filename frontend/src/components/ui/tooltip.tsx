import type {JSX} from 'solid-js'
import {children as resolveChildren, createSignal, Show, splitProps} from 'solid-js'

interface TooltipProps {
    children: JSX.Element
}

export function TooltipProvider(props: TooltipProps) {
    return <>{props.children}</>
}

export function Tooltip(props: TooltipProps) {
    return <>{props.children}</>
}

interface TooltipTriggerProps extends JSX.HTMLAttributes<HTMLDivElement> {
    asChild?: boolean
}

export function TooltipTrigger(props: TooltipTriggerProps) {
    const [local, others] = splitProps(props, ['class', 'children', 'asChild'])
    const resolved = resolveChildren(() => local.children)

    if (local.asChild) {
        return <>{resolved()}</>
    }

    return (
        <div class={local.class} {...others}>
            {resolved()}
        </div>
    )
}

interface TooltipContentProps extends JSX.HTMLAttributes<HTMLDivElement> {
}

export function TooltipContent(props: TooltipContentProps) {
    const [local, others] = splitProps(props, ['class', 'children'])
    const [isVisible, setIsVisible] = createSignal(false)

    return (
        <div
            class="relative inline-block"
            onMouseEnter={() => setIsVisible(true)}
            onMouseLeave={() => setIsVisible(false)}
        >
            <Show when={isVisible()}>
                <div
                    class={`absolute z-50 overflow-hidden rounded-md border border-border bg-popover px-3 py-1.5 text-sm text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95 ${local.class || ''}`}
                    style={{
                        bottom: '100%',
                        left: '50%',
                        transform: 'translateX(-50%)',
                        'margin-bottom': '0.5rem'
                    }}
                    {...others}
                >
                    {local.children}
                </div>
            </Show>
        </div>
    )
}
