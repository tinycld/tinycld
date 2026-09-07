import { Dialog } from '@tinycld/core/ui/dialog'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { useEffect, useRef, useState } from 'react'
import { type TextInput, View } from 'react-native'

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
}

export function PromptDialog({
    isOpen,
    onClose,
    onSubmit,
    title,
    description,
    placeholder,
    confirmLabel = 'Save',
    cancelLabel = 'Cancel',
    defaultValue = '',
    maxLength,
    required = false,
    isSubmitting = false,
}: PromptDialogProps) {
    const [value, setValue] = useState(defaultValue)
    const inputRef = useRef<TextInput>(null)

    useEffect(() => {
        if (isOpen) setValue(defaultValue)
    }, [isOpen, defaultValue])

    // GlueStack's overlay installs its own focus trap after mount and the
    // ModalContent enters with a fade animation, so the input's `autoFocus`
    // prop alone gets clobbered. Imperatively focus on the next frame, once
    // the trap has settled and the content has painted.
    useEffect(() => {
        if (!isOpen) return
        const raf = requestAnimationFrame(() => inputRef.current?.focus())
        return () => cancelAnimationFrame(raf)
    }, [isOpen])

    const trimmed = value.trim()
    const canSubmit = !isSubmitting && (!required || trimmed.length > 0)

    const handleSubmit = () => {
        if (!canSubmit) return
        onSubmit(trimmed)
    }

    return (
        <Dialog isOpen={isOpen} onClose={onClose} title={title} description={description}>
            <Dialog.Body>
                <View
                    className="flex-row border border-border rounded-lg px-3"
                    style={{ paddingVertical: 10 }}
                >
                    <PlainInput
                        ref={inputRef}
                        value={value}
                        onChangeText={setValue}
                        placeholder={placeholder}
                        autoFocus
                        maxLength={maxLength}
                        onSubmitEditing={handleSubmit}
                        editable={!isSubmitting}
                        accessibilityLabel={title}
                        className="flex-1 text-foreground"
                        style={{ fontSize: 15 }}
                    />
                </View>
            </Dialog.Body>
            <Dialog.Footer>
                <Dialog.CancelButton
                    onPress={onClose}
                    label={cancelLabel}
                    isDisabled={isSubmitting}
                />
                <Dialog.ActionButton
                    label={confirmLabel}
                    onPress={handleSubmit}
                    isDisabled={!canSubmit}
                />
            </Dialog.Footer>
        </Dialog>
    )
}
