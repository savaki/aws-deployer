import {createSignal, JSX, Show, children, type Component} from 'solid-js'
import {Portal} from 'solid-js/web'

interface DropdownMenuProps {
    children: JSX.Element
}

interface DropdownMenuTriggerProps {
    children: JSX.Element
    as?: Component<any>
}

interface DropdownMenuContentProps {
    children: JSX.Element
}

interface DropdownMenuItemProps {
    onSelect?: () => void
    disabled?: boolean
    children: JSX.Element
}

interface DropdownMenuContextValue {
    open: () => boolean
    setOpen: (value: boolean) => void
}

let DropdownMenuContext: DropdownMenuContextValue | null = null

export function DropdownMenu(props: DropdownMenuProps) {
    const [open, setOpen] = createSignal(false)

    DropdownMenuContext = {
        open,
        setOpen,
    }

    return <div class="relative inline-block">{props.children}</div>
}

export function DropdownMenuTrigger(props: DropdownMenuTriggerProps) {
    const resolved = children(() => props.children)

    const handleClick = (e: MouseEvent) => {
        e.stopPropagation()
        if (DropdownMenuContext) {
            DropdownMenuContext.setOpen(!DropdownMenuContext.open())
        }
    }

    return (
        <div onClick={handleClick} class="cursor-pointer">
            {resolved()}
        </div>
    )
}

export function DropdownMenuContent(props: DropdownMenuContentProps) {
    const [triggerRef, setTriggerRef] = createSignal<HTMLElement | null>(null)

    const handleClickOutside = (e: MouseEvent) => {
        const target = e.target as Node
        const trigger = triggerRef()
        if (trigger && !trigger.contains(target) && DropdownMenuContext) {
            DropdownMenuContext.setOpen(false)
        }
    }

    // Add click outside listener
    document.addEventListener('click', handleClickOutside)

    return (
        <Show when={DropdownMenuContext?.open()}>
            <Portal>
                <div
                    ref={setTriggerRef}
                    class="absolute z-50 min-w-[8rem] overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md"
                    style={{
                        top: '100%',
                        right: '0',
                        'margin-top': '0.25rem',
                    }}
                >
                    {props.children}
                </div>
            </Portal>
        </Show>
    )
}

export function DropdownMenuItem(props: DropdownMenuItemProps) {
    const handleClick = () => {
        if (!props.disabled && props.onSelect) {
            props.onSelect()
            if (DropdownMenuContext) {
                DropdownMenuContext.setOpen(false)
            }
        }
    }

    return (
        <div
            onClick={handleClick}
            class={`relative flex cursor-pointer select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors ${
                props.disabled
                    ? 'pointer-events-none opacity-50'
                    : 'hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground'
            }`}
        >
            {props.children}
        </div>
    )
}
