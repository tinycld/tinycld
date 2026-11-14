import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ColorPickerGrid, DEFAULT_LABEL_COLOR } from '@tinycld/core/ui/color-picker'
import { Dialog } from '@tinycld/core/ui/dialog'
import { Divider } from '@tinycld/core/ui/divider'
import { type Control, useForm, zodResolver } from '@tinycld/core/ui/form'
import { useLabelMutations } from '@tinycld/core/ui/hooks/useLabelMutations'
import { useLabels } from '@tinycld/core/ui/hooks/useLabels'
import { Check, Plus, Trash2, X } from 'lucide-react-native'
import { useState } from 'react'
import { useController } from 'react-hook-form'
import { Pressable, ScrollView, Text, TextInput, View } from 'react-native'
import { type LabelValues, labelSchema } from './label-schema'

// LabelEditor is the name + color form shared by the edit and create rows. The
// schema's errors show under the row, so a rejected save says why.
function LabelEditor({
    control,
    onSave,
    onCancel,
    isPending,
}: {
    control: Control<LabelValues>
    onSave: () => void
    onCancel: () => void
    isPending: boolean
}) {
    const accentColor = useThemeColor('primary')
    const mutedColor = useThemeColor('muted-foreground')
    const name = useController({ control, name: 'name' })
    const color = useController({ control, name: 'color' })
    return (
        <View className="bg-surface-secondary">
            <View className="flex-row items-center px-3 py-2 gap-2">
                <View
                    className="w-3.5 h-3.5 rounded-full"
                    style={{ backgroundColor: color.field.value }}
                />
                <TextInput
                    // min-w-0 lets the input shrink within the flex row. On web
                    // a raw TextInput keeps its intrinsic <input> width (RN flex
                    // children default to flex-shrink:0), so without this it
                    // overflows the fixed-width modal instead of fitting beside
                    // the swatch and action buttons.
                    className="flex-1 min-w-0 text-sm p-2 border border-border rounded-md bg-background text-foreground"
                    value={name.field.value}
                    onChangeText={name.field.onChange}
                    onBlur={name.field.onBlur}
                    placeholder="Label name"
                    autoFocus
                    onSubmitEditing={onSave}
                />
                <Pressable onPress={onSave} className="p-1.5" disabled={isPending}>
                    <Check size={16} color={accentColor} />
                </Pressable>
                <Pressable onPress={onCancel} className="p-1.5">
                    <X size={16} color={mutedColor} />
                </Pressable>
            </View>
            <FieldError message={name.fieldState.error?.message} />
            <View className="px-1 pb-2">
                <ColorPickerGrid selected={color.field.value} onSelect={color.field.onChange} />
            </View>
            <FieldError message={color.fieldState.error?.message} />
        </View>
    )
}

function FieldError({ message }: { message: string | undefined }) {
    if (!message) return null
    return <Text className="px-3 pb-1 text-xs text-danger">{message}</Text>
}

function LabelRow({
    label,
    isEditing,
    onStartEdit,
    onCancelEdit,
    isConfirmingDelete,
    onStartDelete,
    onCancelDelete,
}: {
    label: { id: string; name: string; color: string }
    isEditing: boolean
    onStartEdit: () => void
    onCancelEdit: () => void
    isConfirmingDelete: boolean
    onStartDelete: () => void
    onCancelDelete: () => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const { updateLabel, deleteLabel } = useLabelMutations()

    const { control, reset, handleSubmit } = useForm({
        resolver: zodResolver(labelSchema),
        mode: 'onChange',
        defaultValues: { name: label.name, color: label.color },
    })

    const onSave = handleSubmit(data => {
        updateLabel.mutate({ id: label.id, ...data }, { onSuccess: onCancelEdit })
    })

    const handleDelete = () => {
        deleteLabel.mutate(label.id)
    }

    // Each edit starts from the stored label, not what a cancelled edit left.
    const handleStartEdit = () => {
        reset({ name: label.name, color: label.color })
        onStartEdit()
    }

    if (isEditing) {
        return (
            <LabelEditor
                control={control}
                onSave={onSave}
                onCancel={onCancelEdit}
                isPending={updateLabel.isPending}
            />
        )
    }

    if (isConfirmingDelete) {
        return (
            <View className="flex-row items-center px-3 py-2 gap-2">
                <View
                    className="w-3.5 h-3.5 rounded-full"
                    style={{ backgroundColor: label.color }}
                />
                <Text numberOfLines={1} className="flex-1 text-sm text-danger">
                    Delete "{label.name}"?
                </Text>
                <Pressable
                    onPress={handleDelete}
                    disabled={deleteLabel.isPending}
                    className="px-2.5 py-1 rounded bg-danger"
                >
                    <Text className="text-xs text-danger-foreground">
                        {deleteLabel.isPending ? 'Deleting' : 'Delete'}
                    </Text>
                </Pressable>
                <Pressable onPress={onCancelDelete} className="px-2 py-1">
                    <Text className="text-xs text-foreground">Cancel</Text>
                </Pressable>
            </View>
        )
    }

    return (
        <Pressable onPress={handleStartEdit}>
            <View className="flex-row items-center px-3 py-2 gap-2.5">
                <View
                    className="w-3.5 h-3.5 rounded-full"
                    style={{ backgroundColor: label.color }}
                />
                <Text numberOfLines={1} className="flex-1 text-sm text-foreground">
                    {label.name}
                </Text>
                <Pressable
                    onPress={e => {
                        e.stopPropagation()
                        onStartDelete()
                    }}
                    className="p-1.5"
                    hitSlop={{ top: 6, bottom: 6, left: 6, right: 6 }}
                >
                    <Trash2 size={14} color={mutedColor} />
                </Pressable>
            </View>
        </Pressable>
    )
}

function CreateLabelRow({ onCreated }: { onCreated: () => void }) {
    const mutedColor = useThemeColor('muted-foreground')
    const { createLabel } = useLabelMutations()
    const [isCreating, setIsCreating] = useState(false)

    const { control, handleSubmit, reset } = useForm({
        resolver: zodResolver(labelSchema),
        mode: 'onChange',
        defaultValues: { name: '', color: DEFAULT_LABEL_COLOR },
    })

    const handleCreate = handleSubmit(data => {
        createLabel.mutate(data, {
            onSuccess: () => {
                reset()
                setIsCreating(false)
                onCreated()
            },
        })
    })

    const handleCancel = () => {
        reset()
        setIsCreating(false)
    }

    if (!isCreating) {
        return (
            <Pressable onPress={() => setIsCreating(true)}>
                <View className="flex-row items-center px-3 py-2 gap-2 opacity-60">
                    <View
                        className="border-[1.5px] border-border/70"
                        style={{
                            width: 14,
                            height: 14,
                            borderRadius: 7,
                            borderStyle: 'dashed',
                            alignItems: 'center',
                            justifyContent: 'center',
                        }}
                    >
                        <Plus size={10} color={mutedColor} />
                    </View>
                    <Text className="text-sm text-muted">New label...</Text>
                </View>
            </Pressable>
        )
    }

    return (
        <LabelEditor
            control={control}
            onSave={handleCreate}
            onCancel={handleCancel}
            isPending={createLabel.isPending}
        />
    )
}

export function LabelManagerPanel({ maxListHeight }: { maxListHeight?: number }) {
    const { labels } = useLabels()
    const [editingLabelId, setEditingLabelId] = useState<string | null>(null)
    const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)

    return (
        <View>
            <View style={{ maxHeight: maxListHeight }}>
                <ScrollView>
                    {labels.length === 0 ? (
                        <View className="py-3 items-center">
                            <Text className="text-xs text-muted">
                                No labels yet — create one below
                            </Text>
                        </View>
                    ) : (
                        labels.map(label => (
                            <LabelRow
                                key={label.id}
                                label={label}
                                isEditing={editingLabelId === label.id}
                                onStartEdit={() => {
                                    setEditingLabelId(label.id)
                                    setConfirmDeleteId(null)
                                }}
                                onCancelEdit={() => setEditingLabelId(null)}
                                isConfirmingDelete={confirmDeleteId === label.id}
                                onStartDelete={() => {
                                    setConfirmDeleteId(label.id)
                                    setEditingLabelId(null)
                                }}
                                onCancelDelete={() => setConfirmDeleteId(null)}
                            />
                        ))
                    )}
                </ScrollView>
            </View>

            <Divider />

            <CreateLabelRow
                onCreated={() => {
                    setEditingLabelId(null)
                    setConfirmDeleteId(null)
                }}
            />
        </View>
    )
}

interface LabelManagerDialogProps {
    isVisible: boolean
    onClose: () => void
}

export function LabelManagerDialog({ isVisible, onClose }: LabelManagerDialogProps) {
    return (
        <Dialog isOpen={isVisible} onClose={onClose} title="Labels">
            <Divider />
            <LabelManagerPanel maxListHeight={320} />
        </Dialog>
    )
}
