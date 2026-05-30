import { FormErrorSummary, TextAreaInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useCallback } from 'react'
import type { NativeSyntheticEvent, TextInputKeyPressEventData } from 'react-native'
import { Pressable, Text, View } from 'react-native'
import { MentionInput, type MentionSuggestion } from './MentionInput'

const composerSchema = z.object({
    body: z.string().trim().min(1, 'Required').max(4000),
})

type ComposerValues = z.infer<typeof composerSchema>

export interface CommentComposerProps {
    placeholder?: string
    submitLabel: string
    isPending?: boolean
    error?: string | null
    autoFocus?: boolean
    onCancel?: () => void
    onSubmit: (body: string) => void
    // When supplied, the composer renders a MentionInput instead of a
    // plain TextAreaInput. The body still flows through the same form
    // value, so onSubmit's signature is unchanged — the only thing
    // that changes is that `[[@id]]` tokens may be embedded in the
    // submitted string.
    mentionSuggestions?: MentionSuggestion[]
    // submitOnEnter switches the composer into chat-style mode:
    // Enter (without Shift) submits the form; Shift+Enter inserts a
    // newline. The visible submit button is hidden — Enter IS the
    // affordance. Used by the suggestion-thread reply composer to
    // match the Google-Docs interaction model where a tiny inline
    // input doesn't warrant a separate Reply button. The Cancel
    // affordance (when onCancel is supplied) still renders.
    submitOnEnter?: boolean
}

// Form composer used inside the drawer (new thread / reply) and any
// other comments surface in the future. Owns its own form state; the
// caller passes raw onSubmit(body) and gets isPending/error feedback in.
// Mentions activate when the caller supplies `mentionSuggestions`.
export function CommentComposer(props: CommentComposerProps) {
    const {
        control,
        handleSubmit,
        reset,
        formState: { errors, isSubmitted },
    } = useForm<ComposerValues>({
        resolver: zodResolver(composerSchema),
        defaultValues: { body: '' },
        mode: 'onChange',
    })

    const onSubmit = useCallback(
        () =>
            handleSubmit(values => {
                props.onSubmit(values.body)
                reset({ body: '' })
            })(),
        [handleSubmit, props.onSubmit, reset]
    )

    // Chat-style key handling for submitOnEnter mode. Enter without
    // shift submits; Shift+Enter falls through to the default newline
    // insertion. We can't easily inspect "is the mention dropdown
    // open" from out here, so a user who hits Enter while the picker
    // is up will both submit AND pick whatever's highlighted — for v1
    // that's acceptable. The picker is click-only today (no keyboard
    // nav), so the typical flow is: type @, click picker, then press
    // Enter to send.
    const onKeyPress = useCallback(
        (e: NativeSyntheticEvent<TextInputKeyPressEventData>) => {
            if (!props.submitOnEnter) return
            const key = e.nativeEvent.key
            // Detect Shift via the underlying DOM event (web). On
            // native there's no Shift modifier in this event, so an
            // Enter on a hardware keyboard would always submit; for
            // the typical native chat case (mobile keyboard with a
            // dedicated "send" button) that's a feature, not a bug.
            const webEvent = (
                e.nativeEvent as unknown as { shiftKey?: boolean; preventDefault?: () => void }
            )
            if (key === 'Enter' && !webEvent.shiftKey) {
                webEvent.preventDefault?.()
                onSubmit()
            }
        },
        [props.submitOnEnter, onSubmit]
    )

    const useMentions = props.mentionSuggestions !== undefined

    return (
        <View>
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            {props.error ? <Text className="text-xs text-danger mb-2">{props.error}</Text> : null}
            {useMentions ? (
                <MentionInput
                    control={control}
                    name="body"
                    placeholder={props.placeholder}
                    autoFocus={props.autoFocus}
                    numberOfLines={3}
                    suggestions={props.mentionSuggestions ?? []}
                    onKeyPress={props.submitOnEnter ? onKeyPress : undefined}
                />
            ) : (
                <TextAreaInput
                    control={control}
                    name="body"
                    placeholder={props.placeholder}
                    autoFocus={props.autoFocus}
                    numberOfLines={3}
                    onKeyPress={props.submitOnEnter ? onKeyPress : undefined}
                />
            )}
            {/* Submit button is hidden in submitOnEnter mode — Enter is the */}
            {/* affordance. Cancel still renders so the user can dismiss the */}
            {/* row without sending. */}
            {(!props.submitOnEnter || props.onCancel) && (
                <View className="flex-row justify-end gap-2 mt-2">
                    {props.onCancel ? (
                        <Pressable
                            accessibilityRole="button"
                            onPress={props.onCancel}
                            accessibilityLabel="Cancel comment"
                            className="px-3 py-1.5 rounded-md"
                        >
                            <Text className="text-xs font-semibold text-muted-foreground">
                                Cancel
                            </Text>
                        </Pressable>
                    ) : null}
                    {!props.submitOnEnter ? (
                        <Pressable
                            accessibilityRole="button"
                            onPress={onSubmit}
                            accessibilityLabel={props.submitLabel}
                            className="px-3 py-1.5 rounded-md bg-primary"
                            disabled={props.isPending}
                            style={{ opacity: props.isPending ? 0.6 : 1 }}
                        >
                            <Text className="text-xs font-semibold text-primary-foreground">
                                {props.submitLabel}
                            </Text>
                        </Pressable>
                    ) : null}
                </View>
            )}
        </View>
    )
}
