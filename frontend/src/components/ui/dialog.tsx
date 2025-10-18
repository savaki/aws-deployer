import type {JSX} from 'solid-js'
import {createEffect, Show, splitProps} from 'solid-js'

interface DialogProps {
    open: boolean
    onOpenChange: (open: boolean) => void
    children: JSX.Element
}

export function Dialog(props: DialogProps) {
    return (
        <Show when={props.open}>
            <div class="fixed inset-0 z-50 flex items-center justify-center">
                {/* Backdrop */}
                <div
                    class="fixed inset-0 bg-black/80"
                    onClick={() => props.onOpenChange(false)}
                />
                {/* Content wrapper */}
                {props.children}
            </div>
        </Show>
    )
}

export function DialogContent(props: JSX.HTMLAttributes<HTMLDivElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])

    createEffect(() => {
        // Prevent body scroll when dialog is open
        document.body.style.overflow = 'hidden'
        return () => {
            document.body.style.overflow = ''
        }
    })

    return (
        <div
            class={`fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg translate-x-[-50%] translate-y-[-50%] gap-2 border border-border bg-background p-3 shadow-lg duration-200 sm:rounded-lg ${local.class || ''}`}
            {...others}
        >
            {local.children}
        </div>
    )
}

export function DialogHeader(props: JSX.HTMLAttributes<HTMLDivElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <div class={`flex flex-col space-y-1.5 text-center sm:text-left ${local.class || ''}`} {...others}>
            {local.children}
        </div>
    )
}

export function DialogTitle(props: JSX.HTMLAttributes<HTMLHeadingElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <h2 class={`text-lg font-semibold leading-none tracking-tight ${local.class || ''}`} {...others}>
            {local.children}
        </h2>
    )
}

export function DialogDescription(props: JSX.HTMLAttributes<HTMLParagraphElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <p class={`text-sm text-muted-foreground ${local.class || ''}`} {...others}>
            {local.children}
        </p>
    )
}

export function DialogFooter(props: JSX.HTMLAttributes<HTMLDivElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <div class={`flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2 ${local.class || ''}`} {...others}>
            {local.children}
        </div>
    )
}
