import { Dialog } from '@tinycld/core/ui/dialog'
import { Kbd } from '@tinycld/core/ui/Kbd'
import { useMemo } from 'react'
import { Text, View } from 'react-native'
import { helpGroups, useShortcutHelp } from './help-list'
import { useShortcutRegistry } from './registry'
import type { Shortcut } from './types'

export { useShortcutHelp, useShortcutHelpStore } from './help-list'

export function ShortcutHelp() {
    const { isOpen, close, scopeFloor } = useShortcutHelp()
    const shortcuts = useShortcutRegistry(s => s.shortcuts)
    const grouped = useMemo(() => helpGroups(shortcuts, scopeFloor), [shortcuts, scopeFloor])
    const groups = Object.keys(grouped)
        .sort()
        .map(name => <HelpGroup key={name} name={name} shortcuts={grouped[name]} />)

    return (
        <Dialog isOpen={isOpen} onClose={close} title="Keyboard shortcuts" size="lg">
            <Dialog.Body>{groups}</Dialog.Body>
        </Dialog>
    )
}

function HelpGroup({ name, shortcuts }: { name: string; shortcuts: Shortcut[] }) {
    const rows = shortcuts.map(s => <HelpRow key={s.id} shortcut={s} />)
    return (
        <View className="mb-4">
            <Text className="text-xs uppercase tracking-wide text-muted-foreground mb-2">
                {name}
            </Text>
            {rows}
        </View>
    )
}

function HelpRow({ shortcut }: { shortcut: Shortcut }) {
    return (
        <View className="flex-row items-center justify-between py-1.5 border-b border-border/30">
            <Text className="text-sm text-foreground">{shortcut.description}</Text>
            <Kbd keys={shortcut.keys} />
        </View>
    )
}
