import { useQueryClient } from '@tanstack/react-query'
import { Avatar, type AvatarImage } from '@tinycld/core/components/Avatar'
import { AvatarCropper } from '@tinycld/core/components/AvatarCropper'
import {
    uploadFormDataWithProgress,
    uploadRecordWithFile,
} from '@tinycld/core/file-viewer/upload-file'
import { usePickFiles } from '@tinycld/core/file-viewer/use-pick-files'
import { type CropRect, parseCrop, serializeCrop } from '@tinycld/core/lib/avatar'
import {
    avatarImageToBlob,
    type PreparedAvatarImage,
    prepareAvatarImage,
} from '@tinycld/core/lib/avatar-upload'
import { captureException } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useOrgBranding } from '@tinycld/core/lib/use-org-branding'
import { ORG_INFO_QUERY_KEY, useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { Dialog } from '@tinycld/core/ui/dialog'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

// The single branding row's id is fixed by its migration — see
// 2020000001_create_org_branding.js — so create and PATCH both target it.
const BRANDING_RECORD_ID = 'branding'

function brandingImage(
    branding: { id: string; logo: string; logo_crop: string } | null
): AvatarImage | undefined {
    if (!branding?.logo) return undefined
    // org_branding's viewRule is public (pre-login screens render the logo
    // before any token exists), so unlike users' avatar this needs no `?token=`.
    const fileUrl = pb.files.getURL(
        { collectionId: 'org_branding', id: branding.id },
        branding.logo
    )
    return { fileUrl, crop: parseCrop(branding.logo_crop) }
}

/**
 * State and mutations for OrgBrandingSection, kept out of the component body
 * for the same reason as AvatarSection's useAvatarEditor. The upload mutation
 * branches on whether a branding row exists yet: the FIRST logo is a create
 * (uploadRecordWithFile), every one after is a PATCH to the fixed row id.
 */
function useOrgBrandingEditor() {
    const { branding, brandingCollection } = useOrgBranding()
    const { org } = useOrgInfo()
    const { pickFiles } = usePickFiles()
    const queryClient = useQueryClient()

    const [cropperImageUri, setCropperImageUri] = useState<string | null>(null)
    const [pendingUpload, setPendingUpload] = useState<PreparedAvatarImage | null>(null)

    // The rail and the sign-in screen read the logo through useOrgInfo's
    // ORG_INFO_QUERY_KEY query, not through org_branding — a session-long
    // cache over a separate unauthenticated endpoint. Every mutation that
    // changes the logo or its crop must invalidate it, or the write succeeds
    // while those surfaces keep showing the stale (or blank) logo.
    const invalidateOrgInfo = () => {
        queryClient.invalidateQueries({ queryKey: ORG_INFO_QUERY_KEY })
    }

    const writeCrop = useMutation({
        mutationFn: mutation(function* (crop: CropRect) {
            yield brandingCollection.update(BRANDING_RECORD_ID, draft => {
                draft.logo_crop = serializeCrop(crop)
            })
        }),
        onSuccess: invalidateOrgInfo,
    })

    const removeLogo = useMutation({
        mutationFn: mutation(function* () {
            yield brandingCollection.update(BRANDING_RECORD_ID, draft => {
                draft.logo = ''
                draft.logo_crop = ''
            })
        }),
        onSuccess: invalidateOrgInfo,
    })

    const uploadLogoBytes = useMutation({
        mutationFn: async (params: PreparedAvatarImage & { crop: CropRect }) => {
            const blob = await avatarImageToBlob(params)
            const ext = params.mimeType.split('/')[1] ?? 'png'
            const file = new File([blob], `logo.${ext}`, { type: params.mimeType })

            if (branding) {
                const formData = new FormData()
                formData.append('logo', file)
                await uploadFormDataWithProgress({
                    url: pb.buildURL(`/api/collections/org_branding/records/${branding.id}`),
                    formData,
                    authToken: pb.authStore.token ?? '',
                    method: 'PATCH',
                })
            } else {
                // No row exists yet — this is the deployment's first logo, so
                // create it. uploadRecordWithFile POSTs (create), which is only
                // correct here because there is nothing to update.
                await uploadRecordWithFile({
                    collection: 'org_branding',
                    fields: { id: BRANDING_RECORD_ID },
                    file: { name: file.name, type: file.type, size: file.size, file },
                    fileField: 'logo',
                })
            }

            await brandingCollection.update(BRANDING_RECORD_ID, draft => {
                draft.logo_crop = serializeCrop(params.crop)
            }).isPersisted.promise
        },
        onSuccess: invalidateOrgInfo,
        onError: err => {
            captureException('settings.branding_upload', err)
            notify.emit({
                event: 'settings.branding_upload_failed',
                title: 'Could not update the logo',
                body: err instanceof Error ? err.message : 'Upload failed',
                data: { error: err instanceof Error ? err.message : String(err) },
            })
        },
    })

    const startUpload = async () => {
        const [picked] = await pickFiles({
            sources: ['photoLibrary', 'documents'],
            multiple: false,
        })
        if (!picked) return
        const prepared = await prepareAvatarImage(picked)
        setPendingUpload(prepared)
        setCropperImageUri(prepared.uri)
    }

    const image = brandingImage(branding)

    const startReposition = () => {
        if (!image?.fileUrl) return
        setPendingUpload(null)
        setCropperImageUri(image.fileUrl)
    }

    const closeCropper = () => {
        setCropperImageUri(null)
        setPendingUpload(null)
    }

    const commitCrop = (crop: CropRect) => {
        if (pendingUpload) {
            uploadLogoBytes.mutate({ ...pendingUpload, crop })
        } else {
            writeCrop.mutate(crop)
        }
        closeCropper()
    }

    return {
        orgName: org?.name ?? '',
        image,
        hasLogo: !!branding?.logo,
        isCropperOpen: cropperImageUri !== null,
        cropperImageUri,
        initialCrop: pendingUpload ? undefined : parseCrop(branding?.logo_crop),
        startUpload,
        startReposition,
        commitCrop,
        closeCropper,
        remove: () => removeLogo.mutate(),
    }
}

export function OrgBrandingSection() {
    const editor = useOrgBrandingEditor()

    return (
        <View className="gap-3">
            <Text className="text-foreground text-xl font-bold">Logo</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-4">
                <Text className="text-[13px] text-muted-foreground">
                    Shown in the package rail and on the sign-in screen. Falls back to your
                    organization's initials when no logo is set.
                </Text>
                <BrandingPreviewRow editor={editor} />
            </View>
            <CropperDialog editor={editor} />
        </View>
    )
}

function BrandingPreviewRow({ editor }: { editor: ReturnType<typeof useOrgBrandingEditor> }) {
    return (
        <View className="flex-row items-center gap-4">
            <Avatar
                testID="avatar-preview"
                name={editor.orgName}
                size={96}
                avatar={editor.image}
                shape="squircle"
            />
            <View className="flex-row gap-2">
                <SmallButton
                    testID="avatar-upload"
                    label="Upload logo"
                    onPress={editor.startUpload}
                />
                <RepositionButton isVisible={editor.hasLogo} onPress={editor.startReposition} />
                <RemoveButton isVisible={editor.hasLogo} onPress={editor.remove} />
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

function CropperDialog({ editor }: { editor: ReturnType<typeof useOrgBrandingEditor> }) {
    if (!editor.isCropperOpen || !editor.cropperImageUri) return null

    return (
        <Dialog isOpen title="Frame your logo" onClose={editor.closeCropper} size="sm">
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
