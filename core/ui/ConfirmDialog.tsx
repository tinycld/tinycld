import { Dialog } from '@tinycld/core/ui/dialog'
import { Text } from 'react-native'

export type ConfirmDialogProps = {
    isOpen: boolean
    onClose: () => void
    onConfirm: () => void
    title: string
    message?: string
    confirmLabel?: string
    cancelLabel?: string
    isDestructive?: boolean
    isSubmitting?: boolean
}

export function ConfirmDialog({
    isOpen,
    onClose,
    onConfirm,
    title,
    message,
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    isDestructive = false,
    isSubmitting = false,
}: ConfirmDialogProps) {
    return (
        // No close button: a confirm is answered, and the two answers are the
        // footer. Escape and the backdrop still dismiss it.
        <Dialog isOpen={isOpen} onClose={onClose} title={title} hasCloseButton={false}>
            <ConfirmMessage message={message} />
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
                />
            </Dialog.Footer>
        </Dialog>
    )
}

function ConfirmMessage({ message }: { message?: string }) {
    if (!message) return null
    return (
        <Dialog.Body>
            <Text className="text-foreground text-sm">{message}</Text>
        </Dialog.Body>
    )
}
