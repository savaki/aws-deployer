import type {JSX} from 'solid-js'
import {splitProps} from 'solid-js'

export function Card(props: JSX.HTMLAttributes<HTMLDivElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <div
            class={`rounded-lg border border-border bg-card text-card-foreground shadow-sm ${local.class || ''}`}
            {...others}
        >
            {local.children}
        </div>
    )
}

export function CardHeader(props: JSX.HTMLAttributes<HTMLDivElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <div class={`flex flex-col space-y-1.5 p-2 ${local.class || ''}`} {...others}>
            {local.children}
        </div>
    )
}

export function CardTitle(props: JSX.HTMLAttributes<HTMLHeadingElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <h3 class={`text-2xl font-semibold leading-none tracking-tight ${local.class || ''}`} {...others}>
            {local.children}
        </h3>
    )
}

export function CardDescription(props: JSX.HTMLAttributes<HTMLParagraphElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <p class={`text-sm text-muted-foreground ${local.class || ''}`} {...others}>
            {local.children}
        </p>
    )
}

export function CardContent(props: JSX.HTMLAttributes<HTMLDivElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <div class={`p-2 ${local.class || ''}`} {...others}>
            {local.children}
        </div>
    )
}

export function CardFooter(props: JSX.HTMLAttributes<HTMLDivElement>) {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <div class={`flex items-center p-2 pt-0 ${local.class || ''}`} {...others}>
            {local.children}
        </div>
    )
}
