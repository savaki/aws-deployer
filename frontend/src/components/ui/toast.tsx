import type {Component, JSX} from 'solid-js'
import {splitProps} from 'solid-js'
import * as ToastPrimitive from '@kobalte/core/toast'
import {IoClose} from 'solid-icons/io'

export const toaster = ToastPrimitive.toaster

type ToastRegionProps = ToastPrimitive.ToastRegionProps & {
    children: JSX.Element
}

export const ToastRegion: Component<ToastRegionProps> = props => {
    return (
        <ToastPrimitive.Region
            {...props}
            class="fixed top-0 z-[100] flex max-h-screen w-full flex-col-reverse p-4 sm:bottom-0 sm:right-0 sm:top-auto sm:flex-col md:max-w-[420px]"
        />
    )
}

export const ToastList: Component<ToastPrimitive.ToastListProps> = props => {
    return <ToastPrimitive.List {...props} class="flex flex-col gap-2" />
}

type ToastRootProps = ToastPrimitive.ToastRootProps & {
    class?: string
    children?: JSX.Element
}

export const Toast: Component<ToastRootProps> = props => {
    const [local, others] = splitProps(props, ['class', 'children'])
    return (
        <ToastPrimitive.Root
            {...others}
            class={`group pointer-events-auto relative flex w-full items-center justify-between space-x-4 overflow-hidden rounded-md border border-border p-6 pr-8 shadow-lg transition-all data-[swipe=cancel]:translate-x-0 data-[swipe=end]:translate-x-[var(--kb-toast-swipe-end-x)] data-[swipe=move]:translate-x-[var(--kb-toast-swipe-move-x)] data-[swipe=move]:transition-none data-[opened]:animate-in data-[closed]:animate-out data-[swipe=end]:animate-out data-[closed]:fade-out-80 data-[closed]:slide-out-to-right-full data-[opened]:slide-in-from-top-full data-[opened]:sm:slide-in-from-bottom-full bg-background ${
                local.class || ''
            }`}
        >
            {local.children}
        </ToastPrimitive.Root>
    )
}

type ToastCloseButtonProps = ToastPrimitive.ToastCloseButtonProps & {
    class?: string
}

export const ToastCloseButton: Component<ToastCloseButtonProps> = props => {
    const [local, others] = splitProps(props, ['class'])
    return (
        <ToastPrimitive.CloseButton
            {...others}
            class={`absolute right-2 top-2 rounded-md p-1 text-foreground/50 opacity-0 transition-opacity hover:text-foreground focus:opacity-100 focus:outline-none focus:ring-2 group-hover:opacity-100 group-[.destructive]:text-red-300 group-[.destructive]:hover:text-red-50 group-[.destructive]:focus:ring-red-400 group-[.destructive]:focus:ring-offset-red-600 ${
                local.class || ''
            }`}
        >
            <IoClose class="h-4 w-4" />
        </ToastPrimitive.CloseButton>
    )
}

type ToastTitleProps = ToastPrimitive.ToastTitleProps & {
    class?: string
}

export const ToastTitle: Component<ToastTitleProps> = props => {
    const [local, others] = splitProps(props, ['class'])
    return (
        <ToastPrimitive.Title
            {...others}
            class={`text-sm font-semibold ${local.class || ''}`}
        />
    )
}

type ToastDescriptionProps = ToastPrimitive.ToastDescriptionProps & {
    class?: string
}

export const ToastDescription: Component<ToastDescriptionProps> = props => {
    const [local, others] = splitProps(props, ['class'])
    return (
        <ToastPrimitive.Description
            {...others}
            class={`text-sm opacity-90 ${local.class || ''}`}
        />
    )
}

export function showToast(props: {
    title?: string
    description?: string
    variant?: 'default' | 'destructive'
    duration?: number
}) {
    // For destructive/error toasts, don't auto-dismiss unless duration is explicitly set
    const duration = props.variant === 'destructive'
        ? (props.duration ?? Infinity)
        : props.duration

    toaster.show(toast => (
        <Toast
            toastId={toast.toastId}
            duration={duration}
            class={props.variant === 'destructive' ? 'destructive group border-destructive bg-destructive text-destructive-foreground' : ''}
        >
            <div class="grid gap-1">
                {props.title && <ToastTitle>{props.title}</ToastTitle>}
                {props.description && <ToastDescription>{props.description}</ToastDescription>}
            </div>
            <ToastCloseButton />
        </Toast>
    ))
}
