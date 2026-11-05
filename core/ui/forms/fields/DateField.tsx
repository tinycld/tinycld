import { MiniCalendar } from '@tinycld/core/components/MiniCalendar'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Popover } from '@tinycld/core/ui/popover'
import { CalendarDays } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, View } from 'react-native'

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
                value={value ?? ''}
                onChangeText={text => onChange(text === '' ? null : text)}
                placeholder={withTime ? 'YYYY-MM-DDTHH:mm' : 'YYYY-MM-DD'}
                placeholderTextColor={muted}
                style={{ color: fg, flex: 1 }}
                editable={!disabled}
                autoFocus={autoFocus && !disabled}
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
                        disabled={disabled}
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
