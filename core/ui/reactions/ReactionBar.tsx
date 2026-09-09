import { type ReactionGroup, reactionKey } from '@tinycld/core/lib/reactions/group'
import { formatReactors } from '@tinycld/core/lib/reactions/names'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { EmojiPicker } from '@tinycld/core/ui/emoji-picker'
import { SmilePlus } from 'lucide-react-native'
import { forwardRef } from 'react'
import { type GestureResponderEvent, Pressable, Text, View } from 'react-native'
import { ReactorTooltip } from './ReactorTooltip'

export interface ReactionBarProps {
    groups: readonly ReactionGroup[]
    /** Prefix for chip test ids: the id of whatever the reactions hang off. */
    targetId: string
    /** False for a reader who may look but not react — a viewer, a share link. */
    canReact: boolean
    onToggle: (emoji: string) => void
    /**
     * Reactor ids -> display names. The package owns this: only it knows which
     * user store to join and what to show for a row the viewer cannot read.
     */
    nameFor: (userId: string) => string
    /** The viewer, so the tooltip can say "You". Empty when anonymous. */
    currentUserId?: string
    /** Beyond this many chips, the rest collapse into a +N marker. */
    maxChips?: number
    /** Display-only: no picker, no toggling. For a dense surface like a tile. */
    readOnly?: boolean
    testIDPrefix?: string
}

/**
 * The row of emoji chips, plus the picker that adds one.
 *
 * Generic on purpose: it takes already-grouped data and reports which emoji
 * was pressed, so it knows nothing about what the reactions hang off. Any
 * package can render its own reactions with it.
 *
 * Nothing at all when there is nothing to show and nothing the reader could
 * add.
 */
export function ReactionBar({
    groups,
    targetId,
    canReact,
    onToggle,
    nameFor,
    currentUserId = '',
    maxChips,
    readOnly = false,
    testIDPrefix = 'reaction',
}: ReactionBarProps) {
    const interactive = canReact && !readOnly
    if (groups.length === 0 && !interactive) return null

    const visible = maxChips === undefined ? groups : groups.slice(0, maxChips)
    const overflow = groups.length - visible.length

    return (
        <View className="flex-row flex-wrap items-center gap-1">
            {visible.map(group => (
                <ReactionChip
                    key={group.emoji}
                    group={group}
                    targetId={targetId}
                    canReact={interactive}
                    onPress={e => {
                        // A bar is almost always inside something else that is
                        // itself pressable — a board tile that opens the card,
                        // a row that opens a thread. Toggling a reaction must
                        // never also trigger that, in any package.
                        e.stopPropagation()
                        onToggle(group.emoji)
                    }}
                    nameFor={nameFor}
                    currentUserId={currentUserId}
                    testIDPrefix={testIDPrefix}
                />
            ))}
            <OverflowChip count={overflow} />
            <PickerSlot isVisible={interactive} onPick={onToggle} testIDPrefix={testIDPrefix} />
        </View>
    )
}

function OverflowChip({ count }: { count: number }) {
    if (count <= 0) return null
    return (
        <View className="rounded-full border border-transparent bg-foreground/[0.06] px-2 py-[2px]">
            <Text className="text-[11.5px] font-medium text-muted">{`+${count}`}</Text>
        </View>
    )
}

function ReactionChip({
    group,
    targetId,
    canReact,
    onPress,
    nameFor,
    currentUserId,
    testIDPrefix,
}: {
    group: ReactionGroup
    targetId: string
    canReact: boolean
    onPress: (event: GestureResponderEvent) => void
    nameFor: (userId: string) => string
    currentUserId: string
    testIDPrefix: string
}) {
    const isOwn = group.ownId !== null
    const tint = isOwn
        ? 'bg-primary/10 border-primary/40'
        : 'bg-foreground/[0.06] border-transparent'
    const testID = `${testIDPrefix}-${targetId}-${reactionKey(group.emoji)}`

    const selfIndex = currentUserId === '' ? -1 : group.userIds.indexOf(currentUserId)
    const tooltip = formatReactors(group.userIds.map(nameFor), group.emoji, { selfIndex })

    const content = (
        <>
            <Text className="text-[12px]">{group.emoji}</Text>
            <Text className={`text-[11.5px] font-medium ${isOwn ? 'text-primary' : 'text-muted'}`}>
                {group.count}
            </Text>
        </>
    )
    const chipClass = `flex-row items-center gap-1 rounded-full border px-2 py-[2px] ${tint}`

    return (
        <ReactorTooltip text={tooltip}>
            {canReact ? (
                <Pressable
                    accessibilityRole="button"
                    accessibilityState={{ selected: isOwn }}
                    accessibilityLabel={tooltip}
                    testID={testID}
                    onPress={onPress}
                    className={`${chipClass} web:outline-none web:focus-visible:ring-2 web:focus-visible:ring-ring`}
                >
                    {content}
                </Pressable>
            ) : (
                <View accessibilityLabel={tooltip} testID={testID} className={chipClass}>
                    {content}
                </View>
            )}
        </ReactorTooltip>
    )
}

function PickerSlot({
    isVisible,
    onPick,
    testIDPrefix,
}: {
    isVisible: boolean
    onPick: (emoji: string) => void
    testIDPrefix: string
}) {
    if (!isVisible) return null
    return (
        <EmojiPicker
            trigger={<AddReactionButton testIDPrefix={testIDPrefix} />}
            onPick={onPick}
            placement="bottom-start"
        />
    )
}

/**
 * forwardRef because Popover clones its trigger to inject onPress and a ref it
 * measures for placement — a wrapper that swallows either leaves the picker
 * unable to open or unable to position itself.
 */
const AddReactionButton = forwardRef<
    View,
    { onPress?: (event: GestureResponderEvent) => void; testIDPrefix?: string }
>(function AddReactionButton({ testIDPrefix = 'reaction', onPress, ...props }, ref) {
    const mutedColor = useThemeColor('muted')
    return (
        <Pressable
            {...props}
            // Same reason as the chip: opening the picker must not also
            // trigger the pressable this bar is sitting inside. Popover
            // injects the onPress that opens it, so this wraps that.
            onPress={event => {
                event.stopPropagation()
                onPress?.(event)
            }}
            ref={ref}
            accessibilityRole="button"
            accessibilityLabel="Add reaction"
            testID={`${testIDPrefix}-add`}
            className="w-6 h-6 items-center justify-center rounded-full border border-dashed border-border hover:border-muted web:outline-none web:focus-visible:ring-2 web:focus-visible:ring-ring"
        >
            <SmilePlus size={13} color={mutedColor} strokeWidth={2.2} />
        </Pressable>
    )
})
