import type { ReactNode } from 'react'
import { Text, View } from 'react-native'

export interface RuleCardProps {
    /** The step name shown top-left: WHEN / IF / THEN. */
    title: string
    /** Dims the card and makes its whole subtree non-interactive. */
    isDisabled?: boolean
    /** Why the card is unusable. Rendered only while disabled. */
    disabledHint?: string
    /** Rendered on the title row's right — ConditionsCard's match pills. */
    trailing?: ReactNode
    children: ReactNode
}

function CardHint({ isVisible, text }: { isVisible: boolean; text: string | undefined }) {
    if (!isVisible || !text) return null
    return <Text className="text-xs text-muted-foreground">{text}</Text>
}

/**
 * The shared shell for the builder's step cards.
 *
 * A disabled card stays MOUNTED and keeps its place in the WHEN → IF → THEN
 * sequence rather than disappearing. Unmounting an unusable step made the cards
 * below it jump up the moment a trigger was picked, and it hid the shape of a
 * rule from someone building their first one; a dimmed card carrying its own
 * reason explains itself instead.
 *
 * Disabling is enforced two ways, and both are needed:
 *
 *   • `pointerEvents="none"` stops touch and mouse.
 *   • the accessibility props take the subtree out of the focus order — on web
 *     `pointerEvents: none` does NOT stop Tab reaching a Pressable, so without
 *     these a keyboard user could still open a menu inside a disabled card
 *     (THEN's add-action menu, which lists nothing until a trigger exists).
 */
export function RuleCard({
    title,
    isDisabled = false,
    disabledHint,
    trailing,
    children,
}: RuleCardProps) {
    return (
        <View
            className={`rounded-xl border p-4 bg-surface-secondary border-border gap-3 ${
                isDisabled ? 'opacity-40' : ''
            }`}
            pointerEvents={isDisabled ? 'none' : 'auto'}
            accessibilityElementsHidden={isDisabled}
            importantForAccessibility={isDisabled ? 'no-hide-descendants' : 'auto'}
        >
            <View className="flex-row items-center justify-between">
                <Text className="text-sm font-semibold text-foreground">{title}</Text>
                {trailing}
            </View>
            <CardHint isVisible={isDisabled} text={disabledHint} />
            {children}
        </View>
    )
}
