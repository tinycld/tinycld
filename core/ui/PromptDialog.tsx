import { useWritesAvailable } from '@tinycld/core/lib/use-writes-available'
import { DIALOG_SUBMIT_SHORTCUT, Dialog } from '@tinycld/core/ui/dialog'
import { Controller, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { useEffect, useRef } from 'react'
import { Text, type TextInputInstance, View } from 'react-native'

export type PromptDialogProps = {
    isOpen: boolean
    onClose: () => void
    onSubmit: (value: string) => void
    title: string
    description?: string
    placeholder?: string
    confirmLabel?: string
    cancelLabel?: string
    defaultValue?: string
    maxLength?: number
    required?: boolean
    isSubmitting?: boolean
    /** A server-side failure of the last submit, shown under the input. */
    errorMessage?: string | null
    /**
     * The submitted value is saved to the server (the default: a rename, a
     * copy, a version), so the submit is disabled while changes cannot be
     * saved. Pass false for a value that stays on the device, such as a link
     * typed into an editor.
     */
    requiresServer?: boolean
    /** The submit button's test id. */
    confirmTestID?: string
}

function promptSchema(required: boolean, maxLength: number | undefined) {
    const base = z.string().trim()
    const bounded = maxLength === undefined ? base : base.max(maxLength)
    return z.object({ value: required ? bounded.min(1) : bounded })
}

export function PromptDialog({
    isOpen,
    onClose,
    title,
    description,
    ...formProps
}: PromptDialogProps) {
    return (
        <Dialog isOpen={isOpen} onClose={onClose} title={title} description={description}>
            <PromptForm isVisible={isOpen} onClose={onClose} title={title} {...formProps} />
        </Dialog>
    )
}

type PromptFormProps = Omit<PromptDialogProps, 'isOpen' | 'description'> & { isVisible: boolean }

// Mounted only while the dialog is open, so each open starts from defaultValue
// rather than whatever was typed before the last cancel.
function PromptForm({ isVisible, ...props }: PromptFormProps) {
    if (!isVisible) return null
    return <PromptFormFields {...props} />
}

function PromptFormFields({
    onClose,
    onSubmit,
    title,
    placeholder,
    confirmLabel = 'Save',
    cancelLabel = 'Cancel',
    defaultValue = '',
    maxLength,
    required = false,
    isSubmitting = false,
    errorMessage,
    requiresServer = true,
    confirmTestID,
}: Omit<PromptFormProps, 'isVisible'>) {
    const inputRef = useRef<TextInputInstance>(null)
    const {
        control,
        handleSubmit,
        formState: { isValid },
    } = useForm({
        resolver: zodResolver(promptSchema(required, maxLength)),
        defaultValues: { value: defaultValue },
        mode: 'onChange',
    })

    // GlueStack's overlay installs its own focus trap after mount and the
    // ModalContent enters with a fade animation, so the input's `autoFocus`
    // prop alone gets clobbered. Imperatively focus on the next frame, once
    // the trap has settled and the content has painted.
    useEffect(() => {
        const raf = requestAnimationFrame(() => inputRef.current?.focus())
        return () => cancelAnimationFrame(raf)
    }, [])

    // The button disables itself; this also stops the Enter key.
    const writes = useWritesAvailable()
    const canSubmit = !isSubmitting && isValid && (!requiresServer || writes.available)

    const submit = () =>
        void handleSubmit(data => {
            if (canSubmit) onSubmit(data.value)
        })()

    return (
        <>
            <Dialog.Body>
                <View
                    className="flex-row border border-border rounded-lg px-3"
                    style={{ paddingVertical: 10 }}
                >
                    <Controller
                        control={control}
                        name="value"
                        render={({ field }) => (
                            <PlainInput
                                ref={inputRef}
                                value={field.value}
                                onChangeText={field.onChange}
                                onBlur={field.onBlur}
                                placeholder={placeholder}
                                autoFocus
                                maxLength={maxLength}
                                onSubmitEditing={submit}
                                editable={!isSubmitting}
                                accessibilityLabel={title}
                                className="flex-1 text-foreground"
                                style={{ fontSize: 15 }}
                            />
                        )}
                    />
                </View>
                <PromptError message={errorMessage} />
            </Dialog.Body>
            <Dialog.Footer>
                <Dialog.CancelButton
                    onPress={onClose}
                    label={cancelLabel}
                    isDisabled={isSubmitting}
                />
                <Dialog.ActionButton
                    label={confirmLabel}
                    onPress={submit}
                    isDisabled={!canSubmit}
                    requiresServer={requiresServer}
                    shortcut={DIALOG_SUBMIT_SHORTCUT}
                    testID={confirmTestID}
                />
            </Dialog.Footer>
        </>
    )
}

function PromptError({ message }: { message?: string | null }) {
    if (!message) return null
    return (
        <Text className="text-xs text-danger mt-2" accessibilityRole="alert">
            {message}
        </Text>
    )
}
