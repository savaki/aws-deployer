import type {JSX} from 'solid-js'
import {createSignal, Show, splitProps} from 'solid-js'
import {IoChevronDown} from 'solid-icons/io'

interface SelectProps {
    value?: string
    onValueChange?: (value: string) => void
    placeholder?: string
    children: JSX.Element
}

interface SelectItemProps {
    value: string
    children: JSX.Element
}

export function Select(props: SelectProps) {
    const [isOpen, setIsOpen] = createSignal(false)

    return (
        <div class="relative">
            <SelectTrigger onClick={() => setIsOpen(!isOpen())} isOpen={isOpen()}>
                <SelectValue value={props.value} placeholder={props.placeholder}/>
            </SelectTrigger>
            <Show when={isOpen()}>
                <SelectContent onClose={() => setIsOpen(false)} onValueChange={props.onValueChange}>
                    {props.children}
                </SelectContent>
            </Show>
        </div>
    )
}

interface SelectTriggerProps extends JSX.HTMLAttributes<HTMLButtonElement> {
    isOpen: boolean
}

export function SelectTrigger(props: SelectTriggerProps) {
    const [local, others] = splitProps(props, ['class', 'children', 'isOpen'])
    return (
        <button
            type="button"
            class={`flex h-8 w-full items-center justify-between rounded-md border border-border bg-background px-2.5 py-1 text-sm ring-offset-background placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 ${local.class || ''}`}
            {...others}
        >
            {local.children}
            <IoChevronDown class={`h-4 w-4 opacity-50 transition-transform ${local.isOpen ? 'rotate-180' : ''}`}/>
        </button>
    )
}

interface SelectValueProps {
    value?: string
    placeholder?: string
}

export function SelectValue(props: SelectValueProps) {
    return (
        <span class="block truncate">
      {props.value || <span class="text-muted-foreground">{props.placeholder}</span>}
    </span>
    )
}

interface SelectContentProps {
    children: JSX.Element
    onClose: () => void
    onValueChange?: (value: string) => void
}

export function SelectContent(props: SelectContentProps) {
    const handleSelect = (value: string) => {
        props.onValueChange?.(value)
        props.onClose()
    }

    return (
        <>
            <div class="fixed inset-0 z-40" onClick={props.onClose}/>
            <div
                class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-md border border-border bg-popover text-popover-foreground shadow-md">
                <div class="p-1">
                    {typeof props.children === 'function'
                        ? props.children
                        : (Array.isArray(props.children)
                                ? (props.children as any[]).map((child: any) =>
                                    typeof child === 'function'
                                        ? child({onSelect: handleSelect})
                                        : child
                                )
                                : props.children
                        )
                    }
                </div>
            </div>
        </>
    )
}

interface SelectItemInternalProps extends SelectItemProps {
    onSelect?: (value: string) => void
}

export function SelectItem(props: SelectItemInternalProps | SelectItemProps) {
    const [local, others] = splitProps(props as SelectItemInternalProps, ['value', 'children', 'onSelect'])

    return (
        <div
            class="relative flex w-full cursor-pointer select-none items-center rounded-sm py-0.5 px-2 text-sm outline-none hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
            onClick={() => local.onSelect?.(local.value)}
            {...others}
        >
            {local.children}
        </div>
    )
}
