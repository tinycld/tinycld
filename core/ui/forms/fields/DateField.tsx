import { MiniCalendar } from '@tinycld/core/components/MiniCalendar'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Popover } from '@tinycld/core/ui/popover'
import { CalendarDays } from 'lucide-react-native'
import { type Ref, useState } from 'react'
import { Pressable, type TextInputInstance, View } from 'react-native'

export interface DateFieldProps {
    /** YYYY-MM-DD, or YYYY-MM-DDTHH:mm when withTime. */
    value: string | null
    onChange: (value: string | null) => void
    withTime: boolean
    disabled?: boolean
    testID?: string
    accessibilityLabel?: string
    /** Focuses the text input when it mounts, so a data-entry form can start on it. */
    autoFocus?: boolean
    /** Called when the text input or the calendar button takes focus (a table notes which row the keyboard is in). */
    onFocus?: () => void
    /** Called when the text input or the calendar button loses focus (a table forgets the row the keyboard left). */
    onBlur?: () => void
    /** The text input, for a caller that moves keyboard focus to it (a table after a row action). */
    inputRef?: Ref<TextInputInstance>
}

// MiniCalendar's day columns are percentage widths, so it has no natural width
// and fills whatever holds it; an unsized popover stretches it across the
// window. 300 gives each column about 39px around its 28px day circle.
const CALENDAR_WIDTH = 300

function toDate(value: string | null): Date {
    if (!value) return new Date()
    const [y, m, d] = value.slice(0, 10).split('-').map(Number)
    const date = new Date(y, m - 1, d)
    return Number.isNaN(date.getTime()) ? new Date() : date
}

function toIso(date: Date, time: string): string {
    const pad = (n: number) => String(n).padStart(2, '0')
    const day = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
    return time ? `${day}T${time}` : day
}

export function DateField({
    value,
    onChange,
    withTime,
    disabled,
    testID,
    accessibilityLabel = 'Date',
    autoFocus = false,
    onFocus,
    onBlur,
    inputRef,
}: DateFieldProps) {
    const [isOpen, setIsOpen] = useState(false)
    const fg = useThemeColor('foreground')
    const muted = useThemeColor('muted-foreground')
    const time = value && value.length > 10 ? value.slice(11, 16) : ''
    const pick = (date: Date) => {
        onChange(toIso(date, withTime ? time || '09:00' : ''))
        setIsOpen(false)
    }
    return (
        <View className="flex-row items-center border border-border rounded-md px-2">
            <PlainInput
                ref={inputRef}
                value={value ?? ''}
                onChangeText={text => onChange(text === '' ? null : text)}
                placeholder={withTime ? 'YYYY-MM-DDTHH:mm' : 'YYYY-MM-DD'}
                placeholderTextColor={muted}
                // minWidth 0: react-native-web's <input> keeps its intrinsic
                // ~150px width otherwise, which pushes the calendar button out
                // of the border in a phone-width slot.
                style={{ color: fg, flex: 1, minWidth: 0 }}
                editable={!disabled}
                autoFocus={autoFocus && !disabled}
                onFocus={onFocus}
                onBlur={onBlur}
                accessibilityLabel={accessibilityLabel}
                testID={testID}
            />
            <Popover
                isOpen={isOpen}
                onOpenChange={setIsOpen}
                presentation="auto"
                width={CALENDAR_WIDTH}
                title="Pick a date"
                trigger={
                    <Pressable
                        className="shrink-0"
                        disabled={disabled}
                        onFocus={onFocus}
                        onBlur={onBlur}
                        accessibilityLabel="Open calendar"
                        testID={testID ? `${testID}-open` : undefined}
                    >
                        <CalendarDays size={18} color={fg} />
                    </Pressable>
                }
            >
                <MiniCalendar selectedDate={toDate(value)} onDateSelect={pick} />
            </Popover>
        </View>
    )
}
