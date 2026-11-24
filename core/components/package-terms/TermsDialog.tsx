import type { PackageTermsEntry } from '@tinycld/core/lib/packages/use-package-terms'
import { Dialog } from '@tinycld/core/ui/dialog'
import { TermsDocument } from './TermsDocument'

interface TermsDialogProps {
    /** The package whose terms show; null keeps the dialog closed. */
    entry: PackageTermsEntry | null
    onClose: () => void
    onDecide: (entry: PackageTermsEntry, decision: 'accepted' | 'declined') => void
    isSubmitting: boolean
}

/**
 * The terms of one package in a dialog (a sheet on a phone): the setup wizard
 * and Settings → Package terms. Accepted terms show with Close only. Agree has
 * no keyboard shortcut on purpose.
 */
export function TermsDialog({ entry, onClose, onDecide, isSubmitting }: TermsDialogProps) {
    if (!entry) return null
    return (
        <Dialog
            isOpen
            onClose={onClose}
            title={`${entry.name}: terms and conditions`}
            size="xl"
            testID="pkg-terms-dialog"
        >
            <Dialog.Body>
                <TermsDocument slug={entry.slug} />
            </Dialog.Body>
            <Dialog.Footer>
                <Dialog.CancelButton
                    onPress={onClose}
                    label={entry.state === 'accepted' ? 'Close' : 'Not now'}
                    isDisabled={isSubmitting}
                    testID="pkg-terms-dialog-close"
                />
                <DecisionButtons entry={entry} onDecide={onDecide} isSubmitting={isSubmitting} />
            </Dialog.Footer>
        </Dialog>
    )
}

function DecisionButtons({
    entry,
    onDecide,
    isSubmitting,
}: {
    entry: PackageTermsEntry
    onDecide: TermsDialogProps['onDecide']
    isSubmitting: boolean
}) {
    if (entry.state === 'accepted') return null
    return (
        <>
            <DeclineAction
                isVisible={!entry.isDeclined}
                isDisabled={isSubmitting}
                onPress={() => onDecide(entry, 'declined')}
            />
            <Dialog.ActionButton
                label="Agree"
                onPress={() => onDecide(entry, 'accepted')}
                isDisabled={isSubmitting}
                testID="pkg-terms-dialog-agree"
            />
        </>
    )
}

function DeclineAction({
    isVisible,
    isDisabled,
    onPress,
}: {
    isVisible: boolean
    isDisabled: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <Dialog.ActionButton
            label="Decline"
            onPress={onPress}
            isDisabled={isDisabled}
            isDestructive
            testID="pkg-terms-dialog-decline"
        />
    )
}
