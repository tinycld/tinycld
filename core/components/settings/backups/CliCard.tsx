import * as Clipboard from 'expo-clipboard'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

const COMMANDS = [
    { id: 'create', command: 'tinycld backup create --out ./backup.age' },
    { id: 'restore', command: 'tinycld backup restore --from ./backup.age' },
] as const

export function CliCard() {
    return (
        <View className="rounded-xl border border-border p-4 gap-3">
            <Text className="text-foreground font-semibold">From the command line</Text>
            <Text className="text-xs text-muted-foreground">
                The CLI writes the archive to a local path instead of a URL, and it can take a
                backup while the app is offline.
            </Text>
            {COMMANDS.map(entry => (
                <CommandLine key={entry.id} id={entry.id} command={entry.command} />
            ))}
        </View>
    )
}

function CommandLine({ id, command }: { id: string; command: string }) {
    const [isCopied, setIsCopied] = useState(false)

    const copy = async () => {
        await Clipboard.setStringAsync(command)
        setIsCopied(true)
    }

    return (
        <View className="flex-row items-center justify-between gap-3">
            <Text className="font-mono text-xs text-foreground flex-1" selectable>
                {command}
            </Text>
            <Pressable testID={`backup-cli-copy-${id}`} onPress={copy}>
                <Text className="text-xs text-primary">{isCopied ? 'Copied!' : 'Copy'}</Text>
            </Pressable>
        </View>
    )
}
