import { eq } from '@tanstack/db'
import { Avatar } from '@tinycld/core/components/Avatar'
import { AvatarCropper } from '@tinycld/core/components/AvatarCropper'
import { uploadFormDataWithProgress } from '@tinycld/core/file-viewer/upload-file'
import { usePickFiles } from '@tinycld/core/file-viewer/use-pick-files'
import { useAuth } from '@tinycld/core/lib/auth'
import { AVATAR_COLORS, type CropRect, parseCrop, serializeCrop } from '@tinycld/core/lib/avatar'
import {
    avatarImageToBlob,
    type PreparedAvatarImage,
    prepareAvatarImage,
} from '@tinycld/core/lib/avatar-upload'
import { captureException } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useAvatarUrl } from '@tinycld/core/lib/use-avatar-url'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { Dialog } from '@tinycld/core/ui/dialog'
import { EmojiPicker } from '@tinycld/core/ui/emoji-picker'
import { Check } from 'lucide-react-native'
import { forwardRef, useState } from 'react'
import type { GestureResponderEvent } from 'react-native'
import { Pressable, Text, View } from 'react-native'

/**
 * All the state and mutations AvatarSection renders from, so the component
 * body stays flat JSX. `cropperImageUri` doubles as the flag for whether the
 * cropper is open and, when set, whether commit re-uploads bytes (a fresh
 * pick) or only writes the crop (reposition of the already-stored photo).
 */
function useAvatarEditor() {
    const { user } = useAuth()
    const [usersCollection] = useStore('users')
    const { pickFiles } = usePickFiles()

    // useAuth's user only carries {id, name, email, isDemo} — the avatar
    // fields live on the full row, read live so a change (incl. one made
    // elsewhere, e.g. an admin clearing an inappropriate photo) reflects here.
    const { data: rows } = useOrgLiveQuery((query, { userId }) =>
        query.from({ users: usersCollection }).where(({ users }) => eq(users.id, userId))
    )
    const profile = rows?.[0]

    const [cropperImageUri, setCropperImageUri] = useState<string | null>(null)
    const [pendingUpload, setPendingUpload] = useState<PreparedAvatarImage | null>(null)

    const writeCrop = useMutation({
        mutationFn: mutation(function* (crop: CropRect) {
            yield usersCollection.update(user.id, draft => {
                draft.avatar_crop = serializeCrop(crop)
            })
        }),
    })

    const writeEmoji = useMutation({
        mutationFn: mutation(function* (glyph: string) {
            yield usersCollection.update(user.id, draft => {
                draft.avatar_emoji = glyph
            })
        }),
    })

    const writeColor = useMutation({
        mutationFn: mutation(function* (color: string) {
            yield usersCollection.update(user.id, draft => {
                draft.avatar_color = color
            })
        }),
    })

    const removeAvatar = useMutation({
        mutationFn: mutation(function* () {
            yield usersCollection.update(user.id, draft => {
                draft.avatar = ''
                draft.avatar_crop = ''
                draft.avatar_emoji = ''
            })
        }),
    })

    const uploadPhotoBytes = useMutation({
        mutationFn: async (params: PreparedAvatarImage & { crop: CropRect }) => {
            const blob = await avatarImageToBlob(params)
            const formData = new FormData()
            formData.append('avatar', blob, `avatar.${params.mimeType.split('/')[1] ?? 'jpg'}`)
            // The user row already exists, so this must PATCH it rather than
            // create a new one — uploadRecordWithFile POSTs to the collection
            // and would create a duplicate row.
            await uploadFormDataWithProgress({
                url: pb.buildURL(`/api/collections/users/records/${user.id}`),
                formData,
                authToken: pb.authStore.token ?? '',
                method: 'PATCH',
            })
            // Bytes land via the raw PATCH above (the sanctioned bypass for file
            // BYTES); the crop rect is an ordinary field and still goes through
            // pbtsdb so optimistic UI and realtime sync see it consistently.
            await usersCollection.update(user.id, draft => {
                draft.avatar_crop = serializeCrop(params.crop)
            }).isPersisted.promise
        },
        onError: err => {
            captureException('settings.avatar_upload', err)
            notify.emit({
                event: 'settings.avatar_upload_failed',
                title: 'Could not update your photo',
                body: err instanceof Error ? err.message : 'Upload failed',
                data: { error: err instanceof Error ? err.message : String(err) },
            })
        },
    })

    const startUpload = async () => {
        const [picked] = await pickFiles({ sources: ['photoLibrary', 'camera'], multiple: false })
        if (!picked) return
        const prepared = await prepareAvatarImage(picked)
        setPendingUpload(prepared)
        setCropperImageUri(prepared.uri)
    }

    const startReposition = () => {
        if (!avatarImage?.sourceUrl) return
        setPendingUpload(null)
        // MUST use sourceUrl, not fileUrl: fileUrl is the 256x256 thumbnail,
        // which PocketBase center-crops to a square. Reopening the cropper on
        // that thumbnail would apply the stored crop rect a second time, on
        // top of a crop that already happened — the subject marches off-frame
        // a little more on every reposition. The un-thumbed original is the
        // only correct source of truth for re-editing.
        setCropperImageUri(avatarImage.sourceUrl)
    }

    const closeCropper = () => {
        setCropperImageUri(null)
        setPendingUpload(null)
    }

    const commitCrop = (crop: CropRect) => {
        if (pendingUpload) {
            uploadPhotoBytes.mutate({ ...pendingUpload, crop })
        } else {
            writeCrop.mutate(crop)
        }
        closeCropper()
    }

    const avatarImage = useAvatarUrl(profile)

    return {
        name: user.name,
        email: user.email,
        avatar: profile?.avatar ?? '',
        avatarColor: profile?.avatar_color ?? '',
        avatarEmoji: profile?.avatar_emoji ?? '',
        avatarImage,
        isCropperOpen: cropperImageUri !== null,
        cropperImageUri,
        initialCrop: pendingUpload ? undefined : parseCrop(profile?.avatar_crop),
        startUpload,
        startReposition,
        commitCrop,
        closeCropper,
        pickEmoji: (glyph: string) => writeEmoji.mutate(glyph),
        pickColor: (color: string) => writeColor.mutate(color),
        remove: () => removeAvatar.mutate(),
    }
}

export function AvatarSection() {
    const editor = useAvatarEditor()

    return (
        <View className="gap-3">
            <Text className="text-foreground text-xl font-bold">Photo</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-4">
                <AvatarPreviewRow editor={editor} />
                <ColorSwatches selected={editor.avatarColor} onSelect={editor.pickColor} />
            </View>
            <CropperDialog editor={editor} />
        </View>
    )
}

function AvatarPreviewRow({ editor }: { editor: ReturnType<typeof useAvatarEditor> }) {
    const { name, email, avatar, avatarColor, avatarEmoji, avatarImage } = editor

    return (
        <View className="flex-row items-center gap-4">
            <Avatar
                testID="avatar-preview"
                name={name}
                email={email}
                size={96}
                avatar={avatarImage}
                emoji={avatarEmoji || undefined}
                color={avatarColor || undefined}
            />
            <View className="gap-2">
                <View className="flex-row gap-2">
                    <SmallButton
                        testID="avatar-upload"
                        label="Upload photo"
                        onPress={editor.startUpload}
                    />
                    <RepositionButton isVisible={!!avatar} onPress={editor.startReposition} />
                </View>
                <View className="flex-row gap-2">
                    <EmojiTrigger onPick={editor.pickEmoji} />
                    <RemoveButton isVisible={!!avatar || !!avatarEmoji} onPress={editor.remove} />
                </View>
            </View>
        </View>
    )
}

function RepositionButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return <SmallButton testID="avatar-reposition" label="Reposition" onPress={onPress} />
}

function RemoveButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return <SmallButton testID="avatar-remove" label="Remove" onPress={onPress} />
}

function EmojiTrigger({ onPick }: { onPick: (glyph: string) => void }) {
    return (
        <EmojiPicker
            trigger={<EmojiTriggerButton />}
            onPick={onPick}
            placement="bottom-start"
            testID="avatar-choose-emoji"
        />
    )
}

/** forwardRef: Popover clones the trigger to inject onPress and a measured ref. */
const EmojiTriggerButton = forwardRef<View, { onPress?: (event: GestureResponderEvent) => void }>(
    function EmojiTriggerButton({ onPress, ...props }, ref) {
        return (
            <Pressable
                {...props}
                ref={ref}
                onPress={onPress}
                testID="avatar-choose-emoji"
                className="rounded-lg px-3 py-2 border border-border"
            >
                <Text className="text-foreground font-semibold">Choose emoji</Text>
            </Pressable>
        )
    }
)

function SmallButton({
    testID,
    label,
    onPress,
}: {
    testID: string
    label: string
    onPress: () => void
}) {
    return (
        <Pressable
            testID={testID}
            onPress={onPress}
            className="rounded-lg px-3 py-2 border border-border"
        >
            <Text className="text-foreground font-semibold">{label}</Text>
        </Pressable>
    )
}

function ColorSwatches({
    selected,
    onSelect,
}: {
    selected: string
    onSelect: (color: string) => void
}) {
    return (
        <View className="gap-2">
            <Text className="text-foreground text-sm font-semibold">Color</Text>
            <View className="flex-row gap-3 flex-wrap">
                {AVATAR_COLORS.map(color => (
                    <ColorSwatch
                        key={color}
                        color={color}
                        isActive={selected === color}
                        onPress={() => onSelect(color)}
                    />
                ))}
            </View>
        </View>
    )
}

function ColorSwatch({
    color,
    isActive,
    onPress,
}: {
    color: string
    isActive: boolean
    onPress: () => void
}) {
    const onSwatchColor = useThemeColor('primary-foreground')
    const borderColor = useThemeColor('border')

    return (
        <Pressable
            testID={`avatar-color-${color}`}
            onPress={onPress}
            className="items-center justify-center"
            style={{
                width: 32,
                height: 32,
                borderRadius: 16,
                backgroundColor: color,
                borderWidth: isActive ? 3 : 1,
                borderColor: isActive ? color : borderColor,
            }}
        >
            <CheckMark isVisible={isActive} color={onSwatchColor} />
        </Pressable>
    )
}

function CheckMark({ isVisible, color }: { isVisible: boolean; color: string }) {
    if (!isVisible) return null
    return <Check size={16} color={color} />
}

function CropperDialog({ editor }: { editor: ReturnType<typeof useAvatarEditor> }) {
    if (!editor.isCropperOpen || !editor.cropperImageUri) return null

    return (
        <Dialog isOpen title="Frame your photo" onClose={editor.closeCropper} size="sm">
            <Dialog.Body>
                <AvatarCropper
                    imageUri={editor.cropperImageUri}
                    initialCrop={editor.initialCrop}
                    onCommit={editor.commitCrop}
                    onCancel={editor.closeCropper}
                />
            </Dialog.Body>
        </Dialog>
    )
}
