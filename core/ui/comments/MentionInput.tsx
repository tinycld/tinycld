import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCallback, useMemo, useRef, useState } from 'react'
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
    // The candidate pool to show. Either a static list the caller
    // already holds, or — the usual case now — the rows a server-side
    // search returned for the query this input reported through
    // `onQueryChange`. Rendered in order, capped at `maxSuggestions`.
    suggestions: MentionSuggestion[]
    // Reports the text typed after `@`, or null when the caret is not
    // inside a fresh `@…` token. The owner keeps it in state and feeds
    // it to a search hook, whose rows come back in `suggestions` — a
    // hook can't run inside this component because the pool belongs to
    // the owning package (text searches users, boards searches project
    // members).
    onQueryChange?: (query: string | null) => void
    numberOfLines?: number
    // Max suggestion rows shown in the popover. Default 6.
    maxSuggestions?: number
}

export function MentionInput<T extends FieldValues = Record<string, unknown>>(
    props: MentionInputProps<T>
) {
    const {
        name,
        control,
        suggestions,
        onQueryChange,
        numberOfLines = 3,
        maxSuggestions = 6,
        ...inputProps
    } = props

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

    // Cap only. The rows in `suggestions` are already the matches for
    // the reported query — a search hook filtered them server-side, and
    // re-filtering here would drop legitimate hits the server matched
    // on a field this component can't see (an email, say). A caller
    // passing a static pool gets an unfiltered list, which is correct
    // for the small fixed rosters that pattern is for.
    const visibleSuggestions = useMemo(
        () => suggestions.slice(0, maxSuggestions),
        [suggestions, maxSuggestions]
    )

    // The current text, mirrored into a ref so the selection handler can
    // read it without being re-created (and re-bound) on every keystroke.
    // A ref, not state: nothing renders off it.
    const textRef = useRef(value)
    textRef.current = value

    const onChangeText = useCallback(
        (next: string) => {
            field.onChange(next)
            textRef.current = next
            // RN fires onSelectionChange for a text change too, but the
            // order differs per platform. Deriving the trigger here
            // against an end-of-text caret covers the case where the
            // change event lands first — which is also the only place a
            // mention is ever begun. The selection event then re-derives
            // it from the settled caret, correcting a mid-body edit.
            reportTrigger(detectTrigger(next, next.length))
        },
        [field, reportTrigger]
    )

    const onSelectionChange = useCallback(
        (e: NativeSyntheticEvent<TextInputSelectionChangeEventData>) => {
            reportTrigger(detectTrigger(textRef.current, e.nativeEvent.selection.start))
        },
        [reportTrigger]
    )

    const onPick = useCallback(
        (s: MentionSuggestion) => {
            if (!trigger) return
            const before = value.slice(0, trigger.atIndex)
            const after = value.slice(trigger.caretIndex)
            const token = `[[@${s.userId}]] `
            const next = `${before}${token}${after}`
            field.onChange(next)
            textRef.current = next
            // The token closes the trigger: no `@…` remains at the caret.
            reportTrigger(null)
            // Re-focus + collapse caret just after the inserted token
            // so subsequent typing happens in the right spot.
            const nextCaret = before.length + token.length
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
    const showSuggestions = !!trigger && visibleSuggestions.length > 0

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
                    {visibleSuggestions.map(s => (
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
