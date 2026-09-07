import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import type { ReactNode } from 'react'
import { useState } from 'react'

interface SuretyGuardProps {
    children: (onOpen: () => void) => ReactNode
    title?: string
    message?: string
    confirmLabel?: string
    onConfirmed: () => void | Promise<void>
}

/** A destructive action behind a confirm: `children` receives the opener. */
export function SuretyGuard({
    children,
    title = 'Are you sure?',
    message = 'This cannot be undone.',
    confirmLabel = 'Yes',
    onConfirmed,
}: SuretyGuardProps) {
    const [open, setOpen] = useState(false)
    const [pending, setPending] = useState(false)

    const handleConfirm = async () => {
        setPending(true)
        try {
            await onConfirmed()
        } finally {
            setPending(false)
            setOpen(false)
        }
    }

    return (
        <>
            {children(() => setOpen(true))}
            <ConfirmDialog
                isOpen={open}
                onClose={() => setOpen(false)}
                onConfirm={handleConfirm}
                title={title}
                message={message}
                confirmLabel={confirmLabel}
                isDestructive
                isSubmitting={pending}
            />
        </>
    )
}

interface ConfirmTrashProps {
    children: (onOpen: () => void) => ReactNode
    itemName: string
    onConfirmed: () => void | Promise<void>
}

export function ConfirmTrash({ children, itemName, onConfirmed }: ConfirmTrashProps) {
    return (
        <SuretyGuard
            title={`Move "${itemName}" to trash?`}
            message="It will be permanently removed after 30 days."
            confirmLabel="Move to trash"
            onConfirmed={onConfirmed}
        >
            {children}
        </SuretyGuard>
    )
}
