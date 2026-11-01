import { Dialog } from '@tinycld/core/ui/dialog'
import type { ReactNode } from 'react'
import { Text } from 'react-native'

export type ConfirmDialogProps = {
    isOpen: boolean
    onClose: () => void
    onConfirm: () => void
    title: string
    message?: string
    /** Extra body content, shown after the message. */
    children?: ReactNode
    confirmLabel?: string
    cancelLabel?: string
    isDestructive?: boolean
    isSubmitting?: boolean
    /**
     * The confirmed action saves to the server (the default: a confirm guards a
     * delete, archive or revoke), so it is disabled while changes cannot be
     * saved. Pass false for a confirm whose action stays on the device.
     */
    requiresServer?: boolean
    /** The confirm button's test id. */
    confirmTestID?: string
}

export function ConfirmDialog({
    isOpen,
    onClose,
    onConfirm,
    title,
    message,
    children,
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    isDestructive = false,
    isSubmitting = false,
    requiresServer = true,
    confirmTestID,
}: ConfirmDialogProps) {
    return (
        // No close button: a confirm is answered, and the two answers are the
        // footer. Escape and the backdrop still dismiss it.
        <Dialog isOpen={isOpen} onClose={onClose} title={title} hasCloseButton={false}>
            <ConfirmBody message={message}>{children}</ConfirmBody>
            <Dialog.Footer>
                <Dialog.CancelButton
                    onPress={onClose}
                    label={cancelLabel}
                    isDisabled={isSubmitting}
                />
                <Dialog.ActionButton
                    label={confirmLabel}
                    onPress={onConfirm}
                    isDisabled={isSubmitting}
                    isDestructive={isDestructive}
                    requiresServer={requiresServer}
                    testID={confirmTestID}
                />
            </Dialog.Footer>
        </Dialog>
    )
}

function ConfirmBody({ message, children }: { message?: string; children?: ReactNode }) {
    if (!message && !children) return null
    return (
        <Dialog.Body testID="confirm-dialog-body">
            <ConfirmText message={message} />
            {children}
        </Dialog.Body>
    )
}

function ConfirmText({ message }: { message?: string }) {
    if (!message) return null
    return <Text className="text-foreground text-sm">{message}</Text>
}
