import { addMonths, getMonthGrid, isSameDay, toDateString } from '@tinycld/core/lib/dates'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronLeft, ChevronRight } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

// Promoted from the calendar package so any package can render a month grid
// without depending on a sibling. Renders on web and native alike — it is
// ordinary RN views, not a native module, which is why the workspace uses this
// rather than one of the platform date pickers (all native-only).

const DAY_LETTERS = [
    { key: 'sun', label: 'S' },
    { key: 'mon', label: 'M' },
    { key: 'tue', label: 'T' },
    { key: 'wed', label: 'W' },
    { key: 'thu', label: 'T' },
    { key: 'fri', label: 'F' },
    { key: 'sat', label: 'S' },
]

const MONTHS = [
    'January',
    'February',
    'March',
    'April',
    'May',
    'June',
    'July',
    'August',
    'September',
    'October',
    'November',
    'December',
]

interface MiniCalendarProps {
    /** The highlighted day. Also picks the month the grid opens on. */
    selectedDate: Date
    onDateSelect: (date: Date) => void
}

export function MiniCalendar({ selectedDate, onDateSelect }: MiniCalendarProps) {
    const [displayMonth, setDisplayMonth] = useState(
        () => new Date(selectedDate.getFullYear(), selectedDate.getMonth(), 1)
    )
    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const activeIndicatorColor = useThemeColor('active-indicator')

    const grid = getMonthGrid(displayMonth)
    const monthLabel = `${MONTHS[displayMonth.getMonth()]} ${displayMonth.getFullYear()}`

    return (
        <View className="px-3 py-2">
            <View className="flex-row justify-between items-center mb-2">
                <Text className="text-foreground" style={{ fontSize: 15, fontWeight: '600' }}>
                    {monthLabel}
                </Text>
                <View className="flex-row gap-2">
                    <Pressable
                        accessibilityRole="button"
                        accessibilityLabel="Previous month"
                        onPress={() => setDisplayMonth(prev => addMonths(prev, -1))}
                        hitSlop={8}
                    >
                        <ChevronLeft size={16} color={mutedColor} />
                    </Pressable>
                    <Pressable
                        accessibilityRole="button"
                        accessibilityLabel="Next month"
                        onPress={() => setDisplayMonth(prev => addMonths(prev, 1))}
                        hitSlop={8}
                    >
                        <ChevronRight size={16} color={mutedColor} />
                    </Pressable>
                </View>
            </View>

            <View className="flex-row">
                {DAY_LETTERS.map(day => (
                    <View key={day.key} className="items-center py-px" style={{ width: '14.28%' }}>
                        <Text
                            className="text-muted-foreground"
                            style={{ fontSize: 12, fontWeight: '600' }}
                        >
                            {day.label}
                        </Text>
                    </View>
                ))}
            </View>

            <View className="flex-row flex-wrap">
                {grid.map(cell => (
                    <DayCell
                        key={toDateString(cell.date)}
                        date={cell.date}
                        isCurrentMonth={cell.isCurrentMonth}
                        isToday={cell.isToday}
                        isSelected={isSameDay(cell.date, selectedDate)}
                        onPress={onDateSelect}
                        colors={{
                            fgColor,
                            mutedColor,
                            primaryColor,
                            primaryFgColor,
                            activeIndicatorColor,
                        }}
                    />
                ))}
            </View>
        </View>
    )
}

interface DayCellProps {
    date: Date
    isCurrentMonth: boolean
    isToday: boolean
    isSelected: boolean
    onPress: (date: Date) => void
    colors: {
        fgColor: string
        mutedColor: string
        primaryColor: string
        primaryFgColor: string
        activeIndicatorColor: string
    }
}

function DayCell({ date, isCurrentMonth, isToday, isSelected, onPress, colors }: DayCellProps) {
    const background = isToday
        ? colors.primaryColor
        : isSelected
          ? `${colors.activeIndicatorColor}30`
          : undefined
    const textColor = isToday
        ? colors.primaryFgColor
        : isCurrentMonth
          ? colors.fgColor
          : colors.mutedColor

    return (
        <Pressable
            accessibilityRole="button"
            accessibilityLabel={toDateString(date)}
            className="items-center py-px"
            style={{ width: '14.28%' }}
            onPress={() => onPress(date)}
        >
            <View
                className="rounded-full items-center justify-center"
                style={{ width: 28, height: 28, backgroundColor: background }}
            >
                <Text
                    style={{
                        fontSize: 13,
                        fontWeight: isToday ? '700' : undefined,
                        color: textColor,
                    }}
                >
                    {date.getDate()}
                </Text>
            </View>
        </Pressable>
    )
}
