import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { ChevronDown } from 'lucide-react-native'
import { Pressable, Text } from 'react-native'

export interface PickItem {
    id: string
    label: string
    isSelected?: boolean
    onSelect: () => void
}

interface PickMenuProps {
    label: string
    /** Says what the menu picks, for a screen reader: "Condition field". */
    purpose: string
    items: PickItem[]
    testID: string
    /** `touch` gives the chip a 44pt-tall target for primary form controls. */
    size?: 'compact' | 'touch'
    /**
     * A disabled chip shows dimmed, reads as disabled to a screen reader, and opens
     * nothing: no press, no web Tab stop, no Enter or Space. A menu that was already open
     * when the chip became disabled shows its rows disabled: none of them picks.
     */
    isDisabled?: boolean
}

const SIZE_CLASS = {
    touch: 'min-w-40 flex-row items-center justify-between gap-2 border border-border rounded-md px-3',
    compact: 'flex-row items-center gap-1 border border-border rounded-md px-2 py-1',
} as const

/** A chip that opens a short menu of choices; each row's test id is `${testID}-${id}`. */
export function PickMenu({
    label,
    purpose,
    items,
    testID,
    size = 'compact',
    isDisabled = false,
}: PickMenuProps) {
    const muted = useThemeColor('muted-foreground')
    // The disabled Pressable drops its press and, on web, its Tab stop (tabIndex -1).
    const className = isDisabled ? `${SIZE_CLASS[size]} opacity-50` : SIZE_CLASS[size]
    const rows = items.map(item => (
        <Menu.Item
            key={item.id}
            label={item.label}
            isSelected={item.isSelected}
            isDisabled={isDisabled}
            onSelect={item.onSelect}
            testID={`${testID}-${item.id}`}
        />
    ))
    const trigger = (
        <Pressable
            className={className}
            style={size === 'touch' ? { minHeight: 44 } : undefined}
            accessibilityLabel={`${purpose}: ${label}`}
            disabled={isDisabled}
            aria-disabled={isDisabled || undefined}
            accessibilityState={{ disabled: isDisabled }}
            testID={testID}
        >
            <Text className="text-foreground text-sm">{label}</Text>
            <ChevronDown size={12} color={muted} />
        </Pressable>
    )
    return (
        <Menu presentation="popover" trigger={trigger}>
            {rows}
        </Menu>
    )
}
