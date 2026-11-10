import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { PickedFile } from '@tinycld/core/file-viewer/picked-file'
import { uploadRecordWithFile } from '@tinycld/core/file-viewer/upload-file'
import { usePickFiles } from '@tinycld/core/file-viewer/use-pick-files'
import { useAuth } from '@tinycld/core/lib/auth'
import { useMutation } from '@tinycld/core/lib/mutations'
import type { FormHost } from '@tinycld/core/lib/packages/derive-form-hosts'
import { collectionByName } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { X } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { Pressable, Text, View } from 'react-native'

export interface FileFieldProps {
    host: FormHost
    fieldKey: string
    recordId: string
    value: string[] | null
    onChange: (ids: string[]) => void
    multiple: boolean
    accept: string[]
    disabled?: boolean
    testID?: string
    accessibilityLabel?: string
}

// The host's row type is only known at runtime (any package table); `column()` reads it by name.
type HostRow = { id: string } & Record<string, unknown>
type FileRow = HostRow

// The file collection is named by the host at runtime, so its columns cannot be keys of any one row type.
function column(ref: object, name: string): string {
    return (ref as Record<string, string>)[name]
}

function useFieldFiles(host: FormHost, fieldKey: string, recordId: string) {
    const files = host.files
    const collection = files ? collectionByName(files.collection) : undefined
    const { data } = useLiveQuery({
        query: query => {
            if (!collection || !files) return null
            return query
                .from({ f: collection })
                .where(({ f }) =>
                    and(
                        eq(column(f, files.recordColumn), recordId),
                        eq(column(f, files.fieldColumn), fieldKey)
                    )
                )
        },
    })
    return { rows: (data ?? []) as FileRow[], collection, files }
}

// Read live: a copy column comes from the host row, which may still be loading when the field mounts.
// Skipped when nothing is copied, so a plain attachment field never subscribes to the host table.
function useHostRecord(host: FormHost, recordId: string, isEnabled: boolean) {
    const collection = isEnabled ? collectionByName(host.collection) : undefined
    const { data } = useLiveQuery({
        query: query =>
            collection
                ? query
                      .from({ r: collection })
                      .where(({ r }) => eq(column(r, 'id'), recordId))
                      .findOne()
                : null,
    })
    return data
}

const isScalar = (v: unknown): v is string | number | boolean =>
    typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean'

/** Only scalar columns copy: an array or object column has no single string form. */
function copiedColumns(record: HostRow | undefined, copyColumns: Record<string, string>) {
    const copied: Record<string, string> = {}
    if (!record) return copied
    for (const [to, from] of Object.entries(copyColumns)) {
        const value = record[from]
        if (isScalar(value)) copied[to] = String(value)
    }
    return copied
}

function FileRowView({
    row,
    nameColumn,
    onRemove,
    disabled,
}: {
    row: FileRow
    nameColumn: string
    onRemove: () => void
    disabled?: boolean
}) {
    const fg = useThemeColor('foreground')
    const label = String(row[nameColumn] ?? row.id)
    return (
        <View className="flex-row items-center py-1">
            <Text className="flex-1 min-w-0 text-foreground">{label}</Text>
            <Pressable
                onPress={onRemove}
                disabled={disabled}
                accessibilityLabel={`Remove ${label}`}
            >
                <X size={16} color={fg} />
            </Pressable>
        </View>
    )
}

function FileRows({
    rows,
    nameColumn,
    onRemove,
    disabled,
}: {
    rows: FileRow[]
    nameColumn: string
    onRemove: (id: string) => void
    disabled?: boolean
}) {
    return (
        <>
            {rows.map(row => (
                <FileRowView
                    key={row.id}
                    row={row}
                    nameColumn={nameColumn}
                    onRemove={() => onRemove(row.id)}
                    disabled={disabled}
                />
            ))}
        </>
    )
}

function pickLabel(isPending: boolean, multiple: boolean): string {
    if (isPending) return 'Uploading…'
    return multiple ? 'Add files' : 'Choose file'
}

export function FileField({
    host,
    fieldKey,
    recordId,
    value,
    onChange,
    multiple,
    accept,
    disabled,
    testID,
    accessibilityLabel,
}: FileFieldProps) {
    const { user } = useAuth()
    const { pickFiles } = usePickFiles()
    const { rows, collection, files } = useFieldFiles(host, fieldKey, recordId)
    const hasCopies = Object.keys(files?.copyColumns ?? {}).length > 0
    const record = useHostRecord(host, recordId, hasCopies)
    const remove = useMutation({
        mutationFn: function* (id: string) {
            if (!collection) return
            yield collection.delete(id)
            onChange((value ?? []).filter(x => x !== id))
        },
    })
    const upload = useMutation({
        mutationFn: async (picked: PickedFile[]) => {
            if (!files) return
            const ids: string[] = []
            for (const file of picked) {
                const id = newRecordId()
                await uploadRecordWithFile({
                    collection: files.collection,
                    fields: {
                        ...copiedColumns(record, files.copyColumns),
                        id,
                        [files.recordColumn]: recordId,
                        [files.fieldColumn]: fieldKey,
                        [files.sizeColumn]: String(file.size),
                        [files.uploaderColumn]: user.id,
                        [files.nameColumn]: file.name,
                    },
                    file,
                    fileField: files.fileColumn,
                })
                ids.push(id)
            }
            if (!multiple) {
                // The single slot is replaced, so the old rows go or the list and the query disagree.
                for (const old of value ?? []) await remove.mutateAsync(old)
            }
            onChange(multiple ? [...(value ?? []), ...ids] : ids.slice(-1))
            await collection?.reload()
        },
    })
    const pick = async () => {
        const picked = await pickFiles({ sources: ['documents'], multiple, mimeTypes: accept })
        if (picked.length > 0) upload.mutate(picked)
    }
    if (!files)
        return <Text className="text-muted-foreground">{host.label} has no file storage.</Text>
    return (
        <View testID={testID} accessibilityLabel={accessibilityLabel}>
            <FileRows
                rows={rows}
                nameColumn={files.nameColumn}
                onRemove={id => remove.mutate(id)}
                disabled={disabled}
            />
            <Button
                onPress={pick}
                isDisabled={disabled || upload.isPending || (!record && hasCopies)}
                size="sm"
                variant="outline"
            >
                <ButtonText>{pickLabel(upload.isPending, multiple)}</ButtonText>
            </Button>
        </View>
    )
}
