import { ResponsiveToolbar, type ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import type { EditorCommands, EditorToolbarState } from '@tinycld/core/lib/editor/types'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { PromptDialog } from '@tinycld/core/ui/PromptDialog'
import { Bold, Italic, Link2, List, ListOrdered, Quote, Underline } from 'lucide-react-native'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Platform, Pressable, View } from 'react-native'

export interface RichTextToolbarProps {
    commands: EditorCommands
    toolbarState: EditorToolbarState
    /** Drawn before the format buttons. */
    leadingItems?: ToolbarItem[]
    /** Drawn after the format buttons, behind a separator. */
    trailingItems?: ToolbarItem[]
    /** Drawn at the row's right edge. */
    rightItems?: ToolbarItem[]
    /** For the wrapping view; the caller sets the border side. */
    className?: string
}

const NO_ITEMS: ToolbarItem[] = []

/**
 * The formatting row of a rich-text editor (`useRichEditor`): Bold, Italic,
 * Underline, the two lists, Blockquote and Link, through `ResponsiveToolbar`,
 * so buttons that do not fit fold into its More menu. Link, and ⌘K while
 * the editor has focus (`commands.subscribeLinkShortcut`), opens a
 * `PromptDialog`: a value sets the link, an empty value removes it.
 */
export function RichTextToolbar({
    commands,
    toolbarState,
    leadingItems = NO_ITEMS,
    trailingItems = NO_ITEMS,
    rightItems,
    className,
}: RichTextToolbarProps) {
    const iconColor = useThemeColor('muted-foreground')
    const activeColor = useThemeColor('primary')
    const [isLinkOpen, setIsLinkOpen] = useState(false)

    const openLink = useCallback(() => setIsLinkOpen(true), [])
    const closeLink = useCallback(() => setIsLinkOpen(false), [])
    useEffect(() => commands.subscribeLinkShortcut?.(openLink), [commands, openLink])
    const applyLink = useCallback(
        (value: string) => {
            if (value) {
                commands.setLink(value)
            } else {
                commands.removeLink()
            }
            setIsLinkOpen(false)
        },
        [commands]
    )

    const items: ToolbarItem[] = useMemo(() => {
        const format: ToolbarItem[] = [
            {
                type: 'custom',
                key: 'bold',
                element: (
                    <FormatButton
                        icon={Bold}
                        label="Bold"
                        isActive={toolbarState.isBoldActive}
                        onPress={() => commands.toggleBold()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: { label: 'Bold', icon: Bold, onPress: () => commands.toggleBold() },
            },
            {
                type: 'custom',
                key: 'italic',
                element: (
                    <FormatButton
                        icon={Italic}
                        label="Italic"
                        isActive={toolbarState.isItalicActive}
                        onPress={() => commands.toggleItalic()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: { label: 'Italic', icon: Italic, onPress: () => commands.toggleItalic() },
            },
            {
                type: 'custom',
                key: 'underline',
                element: (
                    <FormatButton
                        icon={Underline}
                        label="Underline"
                        isActive={toolbarState.isUnderlineActive}
                        onPress={() => commands.toggleUnderline()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: {
                    label: 'Underline',
                    icon: Underline,
                    onPress: () => commands.toggleUnderline(),
                },
            },
            { type: 'separator' },
            {
                type: 'custom',
                key: 'bullet-list',
                element: (
                    <FormatButton
                        icon={List}
                        label="Bullet list"
                        isActive={toolbarState.isBulletListActive}
                        onPress={() => commands.toggleBulletList()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: {
                    label: 'Bullet list',
                    icon: List,
                    onPress: () => commands.toggleBulletList(),
                },
            },
            {
                type: 'custom',
                key: 'ordered-list',
                element: (
                    <FormatButton
                        icon={ListOrdered}
                        label="Numbered list"
                        isActive={toolbarState.isOrderedListActive}
                        onPress={() => commands.toggleOrderedList()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: {
                    label: 'Numbered list',
                    icon: ListOrdered,
                    onPress: () => commands.toggleOrderedList(),
                },
            },
            { type: 'separator' },
            {
                type: 'custom',
                key: 'blockquote',
                element: (
                    <FormatButton
                        icon={Quote}
                        label="Blockquote"
                        isActive={toolbarState.isBlockquoteActive}
                        onPress={() => commands.toggleBlockquote()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: {
                    label: 'Blockquote',
                    icon: Quote,
                    onPress: () => commands.toggleBlockquote(),
                },
            },
            {
                type: 'custom',
                key: 'link',
                element: (
                    <FormatButton
                        icon={Link2}
                        label="Link"
                        isActive={toolbarState.isLinkActive}
                        onPress={openLink}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: { label: 'Link', icon: Link2, onPress: openLink },
            },
        ]
        const trailing: ToolbarItem[] =
            trailingItems.length > 0 ? [{ type: 'separator' }, ...trailingItems] : []
        return [...leadingItems, ...format, ...trailing]
    }, [leadingItems, trailingItems, commands, toolbarState, iconColor, activeColor, openLink])

    const linkDefault = toolbarState.currentLink ?? 'https://'

    return (
        <View className={className}>
            <ResponsiveToolbar items={items} rightItems={rightItems} />
            <PromptDialog
                isOpen={isLinkOpen}
                onClose={closeLink}
                onSubmit={applyLink}
                title="Link"
                placeholder="https://"
                defaultValue={linkDefault}
                confirmLabel="Apply"
                required={false}
                requiresServer={false}
            />
        </View>
    )
}

// On web a mousedown blurs the editor and collapses its selection before the
// press is handled, so a format would apply to nothing. Web-only: native has
// no DOM focus, and React Native rejects the unknown prop.
const FOCUS_GUARD =
    Platform.OS === 'web'
        ? { onMouseDown: (e: { preventDefault: () => void }) => e.preventDefault() }
        : {}

interface FormatButtonProps {
    icon: React.ComponentType<{ size: number; color: string }>
    label: string
    isActive: boolean
    onPress: () => void
    iconColor: string
    activeColor: string
}

function FormatButton({
    icon: Icon,
    label,
    isActive,
    onPress,
    iconColor,
    activeColor,
}: FormatButtonProps) {
    return (
        <Pressable
            className="rounded-md p-1.5"
            style={isActive ? { backgroundColor: `${activeColor}22` } : undefined}
            onPress={onPress}
            accessibilityRole="button"
            accessibilityLabel={label}
            accessibilityState={{ selected: isActive }}
            {...FOCUS_GUARD}
        >
            <Icon size={16} color={isActive ? activeColor : iconColor} />
        </Pressable>
    )
}
