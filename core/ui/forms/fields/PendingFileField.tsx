import { usePickFiles } from '@tinycld/core/file-viewer/use-pick-files'
import { usePendingFiles } from '@tinycld/core/lib/web-forms/pending-files-store'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Pressable, Text, View } from 'react-native'

export interface PendingFileFieldProps {
    value: string | string[] | null
    onChange: (value: string | string[] | null) => void
    multiple: boolean
    accept: string[]
    disabled?: boolean
    testID?: string
    accessibilityLabel?: string
}

function idsOf(value: PendingFileFieldProps['value']): string[] {
    if (!value) return []
    return Array.isArray(value) ? value : [value]
}

// The picker filters by MIME type; extension entries are checked by the server.
function mimeTypesOf(accept: string[]): string[] | undefined {
    const types = accept.filter(a => a.includes('/'))
    return types.length ? types : undefined
}

function usePendingFileField(p: PendingFileFieldProps) {
    const { pickFiles } = usePickFiles()
    const files = usePendingFiles(s => s.files)
    const add = usePendingFiles(s => s.add)
    const remove = usePendingFiles(s => s.remove)
    const ids = idsOf(p.value)
    const emit = (next: string[]) => {
        if (p.multiple) p.onChange(next.length ? next : null)
        else p.onChange(next[0] ?? null)
    }
    return {
        rows: ids.map(id => ({ id, name: files[id]?.name ?? '' })),
        pick: async () => {
            const picked = await pickFiles({
                multiple: p.multiple,
                mimeTypes: mimeTypesOf(p.accept),
                sources: ['documents', 'photoLibrary'],
            })
            if (picked.length === 0) return
            const added = add(picked)
            if (!p.multiple) for (const old of ids) remove(old)
            emit(p.multiple ? [...ids, ...added] : added.slice(-1))
        },
        drop: (id: string) => {
            remove(id)
            emit(ids.filter(i => i !== id))
        },
    }
}

function FileRow({
    name,
    onRemove,
    testID,
}: {
    name: string
    onRemove: () => void
    testID: string
}) {
    return (
        <View className="flex-row items-center gap-2 py-1">
            <Text className="text-foreground flex-1" numberOfLines={1}>
                {name}
            </Text>
            <Pressable onPress={onRemove} accessibilityLabel={`Remove ${name}`} testID={testID}>
                <Text className="text-muted-foreground">Remove</Text>
            </Pressable>
        </View>
    )
}

export function PendingFileField(p: PendingFileFieldProps) {
    const f = usePendingFileField(p)
    const rows = f.rows.map(r => (
        <FileRow
            key={r.id}
            name={r.name}
            onRemove={() => f.drop(r.id)}
            testID={`${p.testID}-remove`}
        />
    ))
    return (
        <View>
            {rows}
            <Button
                variant="outline"
                size="sm"
                className="self-start"
                onPress={f.pick}
                isDisabled={p.disabled}
                testID={`${p.testID}-pick`}
                accessibilityLabel={p.accessibilityLabel}
            >
                <ButtonText>{p.multiple ? 'Add files' : 'Choose file'}</ButtonText>
            </Button>
        </View>
    )
}
