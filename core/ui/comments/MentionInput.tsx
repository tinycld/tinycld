import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCallback, useRef, useState } from 'react'
import { type Control, type FieldValues, type Path, useController } from 'react-hook-form'
import type { TextInputProps as RNTextInputProps } from 'react-native'
import {
    type NativeSyntheticEvent,
    Pressable,
    TextInput as RNTextInput,
    Text,
    type TextInputSelectionChangeEventData,
    View,
} from 'react-native'
import { detectTrigger, type MentionTrigger } from './mention-input-helpers'

// A composer textarea that autocompletes @-mentions. The control is
// react-hook-form-aware (matches TextAreaInput's shape) so it slots
// straight into CommentComposer / NewCommentButton without a parallel
// form pipeline.
//
// Wire format: when the user picks a suggestion, the bare `@query`
// they typed is replaced with the literal token `[[@<userId>]]`
// followed by a space. The token is what gets stored in the comment
// body — `parseMentions` (lib/comments/mentions.ts) extracts it later
// for the comment_mentions rows. Read-mode rendering is the consumer's
// concern (they have access to the user id → display name map).
//
// Why not parse the token in this component: rendering tokens as
// pretty pills inside a controlled <TextInput> means re-implementing
// caret positioning + cursor jumps across mark boundaries on every
// platform. v1 ships the raw token visible in the input; we trade
// a bit of in-input ugliness for a control flow that's actually
// readable and doesn't break the form value contract.

export interface MentionSuggestion {
    userId: string
    displayName: string
    // Optional secondary line — caller often wants to surface an
    // email or role here. Falsy values are ignored.
    secondary?: string
}

export type MentionInputProps<T extends FieldValues = Record<string, unknown>> = Omit<
    RNTextInputProps,
    'value' | 'onChangeText' | 'onBlur' | 'onSelectionChange'
> & {
    name: Path<T>
    control: Control<T>
    // The candidate pool to show, rendered in order, all of it. Either
    // a static list the caller already holds, or — the usual case now —
    // the rows a server-side search returned for the query this input
    // reported through `onQueryChange`.
    //
    // There is deliberately no cap here. Whoever produces the list
    // already bounds it (`useMentionCandidates` does it in the request,
    // via `.limit(MENTION_CANDIDATE_LIMIT)`), and a second cap in the
    // view only means the popover silently shows fewer rows than were
    // fetched and paid for — the old default of 6 discarded 14 of the
    // 20 that came back. One bound, at the source.
    suggestions: MentionSuggestion[]
    // Reports the text typed after `@`, or null when the caret is not
    // inside a fresh `@…` token. The owner keeps it in state and feeds
    // it to a search hook, whose rows come back in `suggestions` — a
    // hook can't run inside this component because the pool belongs to
    // the owning package (text searches users, boards searches project
    // members).
    onQueryChange?: (query: string | null) => void
    numberOfLines?: number
}

export function MentionInput<T extends FieldValues = Record<string, unknown>>(
    props: MentionInputProps<T>
) {
    const { name, control, suggestions, onQueryChange, numberOfLines = 3, ...inputProps } = props

    const {
        field,
        fieldState: { error },
    } = useController({ name, control })

    const placeholderColor = useThemeColor('field-placeholder')
    const inputRef = useRef<RNTextInput | null>(null)

    const value: string = field.value || ''

    // The trigger is derived from (text, caret) — both of which only
    // ever change in an event handler, so it is recomputed there and
    // held in state rather than during render. That is what lets the
    // input report the live query to its owner: a render pass must not
    // call a parent's setter, an event handler may.
    const [trigger, setTrigger] = useState<MentionTrigger | null>(null)

    const reportTrigger = useCallback(
        (next: MentionTrigger | null) => {
            setTrigger(next)
            onQueryChange?.(next ? next.query : null)
        },
        [onQueryChange]
    )

    // Last known (text, caret) pair. A ref, not state: nothing renders
    // off it, and both handlers need the other's latest value to derive
    // the trigger without being re-bound on every keystroke.
    const lastRef = useRef({ text: value, caret: value.length })
    if (lastRef.current.text !== value) {
        // The value moved without going through onChangeText — a form
        // reset after submit, or a caller writing the field directly.
        // Resync so the next edit's caret arithmetic starts from the
        // text actually on screen, and clamp the caret into it.
        lastRef.current = { text: value, caret: Math.min(lastRef.current.caret, value.length) }
    }

    const onChangeText = useCallback(
        (next: string) => {
            field.onChange(next)
            // RN fires onSelectionChange for a text change too, but the
            // order differs per platform, so the settled caret may not
            // have arrived yet. Carry the last known caret through the
            // edit instead of assuming end-of-text: an insertion of
            // (next.length - prev.length) characters at the caret moves
            // it by exactly that much. Assuming end-of-text instead
            // opens the popover on an unrelated `@…` further along the
            // body — and a pick would then splice at that stale offset
            // and silently truncate everything between.
            const prev = lastRef.current
            const caret = Math.min(prev.caret + (next.length - prev.text.length), next.length)
            lastRef.current = { text: next, caret }
            reportTrigger(detectTrigger(next, caret))
        },
        [field, reportTrigger]
    )

    const onSelectionChange = useCallback(
        (e: NativeSyntheticEvent<TextInputSelectionChangeEventData>) => {
            const caret = e.nativeEvent.selection.start
            lastRef.current = { ...lastRef.current, caret }
            reportTrigger(detectTrigger(lastRef.current.text, caret))
        },
        [reportTrigger]
    )

    const onPick = useCallback(
        (s: MentionSuggestion) => {
            if (!trigger) return
            // Re-validate the trigger against the body it is about to
            // splice. The trigger was derived in an earlier event, and
            // the value can have moved since (a pick raced against the
            // selection event, or the form was reset under us). Splicing
            // on stale offsets silently deletes whatever now sits
            // between them, so a mismatch closes the popover instead.
            const isStale =
                trigger.caretIndex > value.length ||
                value.slice(trigger.atIndex, trigger.caretIndex) !== `@${trigger.query}`
            if (isStale) {
                reportTrigger(null)
                return
            }
            const before = value.slice(0, trigger.atIndex)
            const after = value.slice(trigger.caretIndex)
            const token = `[[@${s.userId}]] `
            const next = `${before}${token}${after}`
            field.onChange(next)
            // Re-focus + collapse caret just after the inserted token
            // so subsequent typing happens in the right spot.
            const nextCaret = before.length + token.length
            lastRef.current = { text: next, caret: nextCaret }
            // The token closes the trigger: no `@…` remains at the caret.
            reportTrigger(null)
            // RN TextInput doesn't honor a programmatic `selection`
            // prop while the user holds focus on some platforms; the
            // imperative ref call covers iOS/Android. Web's react-
            // native-web maps both to the underlying <textarea>.
            inputRef.current?.setNativeProps?.({
                selection: { start: nextCaret, end: nextCaret },
            })
        },
        [trigger, value, field, reportTrigger]
    )

    const hasError = !!error
    const showSuggestions = !!trigger && suggestions.length > 0

    return (
        <View>
            <RNTextInput
                ref={inputRef}
                multiline
                numberOfLines={numberOfLines}
                value={value}
                onChangeText={onChangeText}
                onBlur={field.onBlur}
                onSelectionChange={onSelectionChange}
                accessibilityLabel={name}
                testID={name}
                placeholder={inputProps.placeholder}
                placeholderTextColor={placeholderColor}
                textAlignVertical="top"
                className={`border rounded-lg px-3 py-2.5 text-base text-foreground bg-background ${
                    hasError ? 'border-danger' : 'border-border'
                }`}
                style={{ minHeight: numberOfLines * 24 }}
                {...inputProps}
            />
            {showSuggestions ? (
                <View className="mt-1 border border-border rounded-md bg-background overflow-hidden">
                    {suggestions.map(s => (
                        <Pressable
                            key={s.userId}
                            onPress={() => onPick(s)}
                            accessibilityLabel={`Mention ${s.displayName}`}
                            className="px-3 py-2 border-b border-border"
                        >
                            <Text className="text-sm text-foreground">{s.displayName}</Text>
                            {s.secondary ? (
                                <Text className="text-xs text-muted-foreground">{s.secondary}</Text>
                            ) : null}
                        </Pressable>
                    ))}
                </View>
            ) : null}
            {hasError ? <Text className="text-xs text-danger mt-1">{error.message}</Text> : null}
        </View>
    )
}
