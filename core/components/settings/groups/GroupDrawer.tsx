import { Avatar } from '@tinycld/core/components/Avatar'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useGroupMembersAdmin, useGroupsAdmin } from '@tinycld/core/lib/groups/use-groups-admin'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import {
    Drawer,
    DrawerBackdrop,
    DrawerBody,
    DrawerCloseButton,
    DrawerContent,
    DrawerFooter,
    DrawerHeader,
} from '@tinycld/core/ui/drawer'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Search, Trash2, UserPlus, X } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

export type GroupDrawerMode =
    | { kind: 'closed' }
    | { kind: 'create' }
    | { kind: 'view'; groupId: string }

interface GroupDrawerProps {
    mode: GroupDrawerMode
    onClose: () => void
    onCreated: (groupId: string) => void
}

const groupSchema = z.object({
    name: z
        .string()
        .trim()
        .min(1, 'Give the group a name')
        .max(100, 'Keep it under 100 characters'),
    description: z.string().trim().max(500, 'Keep it under 500 characters'),
})
type GroupFormValues = z.infer<typeof groupSchema>

export function GroupDrawer({ mode, onClose, onCreated }: GroupDrawerProps) {
    const isOpen = mode.kind !== 'closed'
    return (
        <Drawer isOpen={isOpen} onClose={onClose} anchor="right" size="md">
            <DrawerBackdrop />
            <DrawerContent>
                <CreateView
                    isVisible={mode.kind === 'create'}
                    onClose={onClose}
                    onCreated={onCreated}
                />
                <ViewGroup groupId={mode.kind === 'view' ? mode.groupId : ''} onClose={onClose} />
            </DrawerContent>
        </Drawer>
    )
}

function useGroupForm(defaults: GroupFormValues) {
    return useForm<GroupFormValues>({
        mode: 'onChange',
        resolver: zodResolver(groupSchema),
        defaultValues: defaults,
    })
}

function CreateView({
    isVisible,
    onClose,
    onCreated,
}: {
    isVisible: boolean
    onClose: () => void
    onCreated: (id: string) => void
}) {
    if (!isVisible) return null
    return <CreateViewOpen onClose={onClose} onCreated={onCreated} />
}

function CreateViewOpen({
    onClose,
    onCreated,
}: {
    onClose: () => void
    onCreated: (id: string) => void
}) {
    const { createGroup } = useGroupsAdmin()
    const form = useGroupForm({ name: '', description: '' })
    const [isSaving, setIsSaving] = useState(false)
    const onSubmit = form.handleSubmit(async values => {
        setIsSaving(true)
        try {
            const id = await createGroup(values)
            onCreated(id)
        } catch (error) {
            handleMutationErrorsWithForm({ setError: form.setError, getValues: form.getValues })(
                error
            )
        } finally {
            setIsSaving(false)
        }
    })
    return (
        <>
            <Header
                title="New group"
                subtitle="Share with everyone in it at once"
                onClose={onClose}
            />
            <DrawerBody>
                <GroupFields form={form} />
            </DrawerBody>
            <DrawerFooter>
                <FooterActions
                    primaryLabel={isSaving ? 'Creating…' : 'Create group'}
                    primaryTestID="group-create-submit"
                    isDisabled={isSaving || !form.formState.isValid}
                    onCancel={onClose}
                    onPrimary={onSubmit}
                />
            </DrawerFooter>
        </>
    )
}

function ViewGroup({ groupId, onClose }: { groupId: string; onClose: () => void }) {
    if (!groupId) return null
    return <ViewGroupOpen groupId={groupId} onClose={onClose} />
}

function ViewGroupOpen({ groupId, onClose }: { groupId: string; onClose: () => void }) {
    const { groups, updateGroup, deleteGroup, isPending } = useGroupsAdmin()
    const group = groups.find(g => g.id === groupId)
    const form = useGroupForm({ name: group?.name ?? '', description: group?.description ?? '' })
    const [isDeleting, setIsDeleting] = useState(false)
    const onSave = form.handleSubmit(values => updateGroup(groupId, values))

    return (
        <>
            <Header
                title={group?.name ?? 'Group'}
                subtitle="Members and details"
                onClose={onClose}
            />
            <DrawerBody>
                <View className="gap-5">
                    <GroupFields form={form} />
                    <SaveRow
                        isVisible={form.formState.isDirty}
                        isDisabled={isPending || !form.formState.isValid}
                        onPress={onSave}
                    />
                    <MembersSection groupId={groupId} />
                </View>
            </DrawerBody>
            <DrawerFooter>
                <DeleteRow name={group?.name ?? ''} onPress={() => setIsDeleting(true)} />
            </DrawerFooter>
            <ConfirmDialog
                isOpen={isDeleting}
                onClose={() => setIsDeleting(false)}
                onConfirm={() => {
                    deleteGroup(groupId)
                    setIsDeleting(false)
                    onClose()
                }}
                title={`Delete "${group?.name ?? ''}"?`}
                message="Every share granted to this group is removed. Members keep anything shared with them directly."
                confirmLabel="Delete group"
                isDestructive
                isSubmitting={isPending}
            />
        </>
    )
}

function Header({
    title,
    subtitle,
    onClose,
}: {
    title: string
    subtitle: string
    onClose: () => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <DrawerHeader>
            <View className="flex-1 gap-0.5">
                <Text className="text-foreground" style={{ fontSize: 17, fontWeight: '700' }}>
                    {title}
                </Text>
                <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                    {subtitle}
                </Text>
            </View>
            <DrawerCloseButton onPress={onClose}>
                <X size={18} color={mutedColor} />
            </DrawerCloseButton>
        </DrawerHeader>
    )
}

function GroupFields({ form }: { form: ReturnType<typeof useGroupForm> }) {
    return (
        <View className="gap-4">
            <FormErrorSummary
                errors={form.formState.errors}
                isEnabled={form.formState.isSubmitted}
            />
            <TextInput control={form.control} name="name" label="Name" placeholder="Sales" />
            <TextInput
                control={form.control}
                name="description"
                label="Description"
                placeholder="Optional"
            />
        </View>
    )
}

function SaveRow({
    isVisible,
    isDisabled,
    onPress,
}: {
    isVisible: boolean
    isDisabled: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <View className="flex-row justify-end">
            <PrimaryButton
                label="Save"
                testID="group-save"
                isDisabled={isDisabled}
                onPress={onPress}
            />
        </View>
    )
}

function MembersSection({ groupId }: { groupId: string }) {
    const { members, candidates, addMember, removeMember, isPending } =
        useGroupMembersAdmin(groupId)
    const [query, setQuery] = useState('')
    const needle = query.trim().toLowerCase()
    const shown = candidates
        .filter(
            c =>
                !needle ||
                c.name.toLowerCase().includes(needle) ||
                c.email.toLowerCase().includes(needle)
        )
        .slice(0, 20)
    const mutedColor = useThemeColor('muted')

    return (
        <View className="gap-3">
            <Text className="text-[12px] font-semibold text-muted uppercase tracking-wide">
                Members
            </Text>
            <View className="rounded-xl border border-border overflow-hidden">
                {members.map((member, index) => (
                    <View
                        key={member.membershipId}
                        className={index > 0 ? 'border-t border-border' : ''}
                    >
                        <MemberRow
                            userId={member.userId}
                            name={member.name}
                            email={member.email}
                            onRemove={() => removeMember(member.membershipId)}
                        />
                    </View>
                ))}
                <NoMembersNote isVisible={members.length === 0} />
            </View>
            <View className="flex-row items-center gap-2 px-3 py-2 rounded-md border border-border bg-background">
                <Search size={15} color={mutedColor} strokeWidth={2.2} />
                <PlainInput
                    testID="group-add-member-search"
                    value={query}
                    onChangeText={setQuery}
                    placeholder="Add a person by name or email"
                    placeholderTextColor={mutedColor}
                    className="flex-1 text-[13.5px] text-foreground"
                />
            </View>
            <CandidateRows
                isVisible={needle.length > 0}
                candidates={shown}
                isPending={isPending}
                onAdd={addMember}
            />
        </View>
    )
}

function MemberRow({
    userId,
    name,
    email,
    onRemove,
}: {
    userId: string
    name: string
    email: string
    onRemove: () => void
}) {
    const dangerColor = useThemeColor('danger')
    return (
        <View
            testID={`group-member-row-${email}`}
            className="flex-row items-center gap-3 py-2.5 px-3"
        >
            <Avatar name={name || email} email={email} colorKey={userId} size={28} />
            <View className="flex-1 min-w-0">
                <Text className="text-[13.5px] font-medium text-foreground" numberOfLines={1}>
                    {name || email}
                </Text>
                <Text className="text-[12px] text-muted" numberOfLines={1}>
                    {email}
                </Text>
            </View>
            <Pressable
                accessibilityRole="button"
                accessibilityLabel={`Remove ${name || email} from group`}
                onPress={onRemove}
                hitSlop={8}
                className="p-1.5 rounded-md"
            >
                <X size={16} color={dangerColor} strokeWidth={2.2} />
            </Pressable>
        </View>
    )
}

function NoMembersNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="px-3 py-3">
            <Text className="text-[12px] text-muted">No members yet.</Text>
        </View>
    )
}

function CandidateRows({
    isVisible,
    candidates,
    isPending,
    onAdd,
}: {
    isVisible: boolean
    candidates: { userId: string; name: string; email: string }[]
    isPending: boolean
    onAdd: (userId: string) => void
}) {
    const primaryFgColor = useThemeColor('primary-foreground')
    if (!isVisible) return null
    if (candidates.length === 0)
        return <Text className="text-[12px] text-muted px-1">No matching people</Text>
    return (
        <View className="rounded-xl border border-border overflow-hidden">
            {candidates.map((candidate, index) => (
                <View key={candidate.userId} className={index > 0 ? 'border-t border-border' : ''}>
                    <View
                        testID={`group-candidate-row-${candidate.email}`}
                        className="flex-row items-center gap-3 py-2.5 px-3"
                    >
                        <View className="flex-1 min-w-0">
                            <Text
                                className="text-[13.5px] font-medium text-foreground"
                                numberOfLines={1}
                            >
                                {candidate.name || candidate.email}
                            </Text>
                            <Text className="text-[12px] text-muted" numberOfLines={1}>
                                {candidate.email}
                            </Text>
                        </View>
                        <Pressable
                            accessibilityRole="button"
                            accessibilityLabel={`Add ${candidate.name || candidate.email}`}
                            disabled={isPending}
                            onPress={() => onAdd(candidate.userId)}
                            className="flex-row items-center gap-1.5 rounded-md bg-primary"
                            style={{
                                paddingVertical: 6,
                                paddingHorizontal: 10,
                                opacity: isPending ? 0.5 : 1,
                            }}
                        >
                            <UserPlus size={13} color={primaryFgColor} />
                            <Text
                                className="text-primary-foreground"
                                style={{ fontSize: 12, fontWeight: '700' }}
                            >
                                Add
                            </Text>
                        </Pressable>
                    </View>
                </View>
            ))}
        </View>
    )
}

function DeleteRow({ name, onPress }: { name: string; onPress: () => void }) {
    const dangerColor = useThemeColor('danger')
    return (
        <Pressable
            testID="group-delete"
            accessibilityRole="button"
            accessibilityLabel={`Delete group ${name}`}
            onPress={onPress}
            className="flex-row items-center gap-2 rounded-md"
            style={{ paddingVertical: 8, paddingHorizontal: 12 }}
        >
            <Trash2 size={14} color={dangerColor} />
            <Text className="text-danger" style={{ fontSize: 13, fontWeight: '600' }}>
                Delete group
            </Text>
        </Pressable>
    )
}

function FooterActions({
    primaryLabel,
    primaryTestID,
    isDisabled,
    onCancel,
    onPrimary,
}: {
    primaryLabel: string
    primaryTestID: string
    isDisabled: boolean
    onCancel: () => void
    onPrimary: () => void
}) {
    return (
        <View className="flex-row items-center justify-end gap-2">
            <Pressable
                onPress={onCancel}
                className="rounded-md"
                style={{ paddingVertical: 8, paddingHorizontal: 14 }}
            >
                <Text className="text-muted-foreground" style={{ fontSize: 13, fontWeight: '600' }}>
                    Cancel
                </Text>
            </Pressable>
            <PrimaryButton
                label={primaryLabel}
                testID={primaryTestID}
                isDisabled={isDisabled}
                onPress={onPrimary}
            />
        </View>
    )
}

function PrimaryButton({
    label,
    testID,
    isDisabled,
    onPress,
}: {
    label: string
    testID: string
    isDisabled: boolean
    onPress: () => void
}) {
    return (
        <Pressable
            testID={testID}
            onPress={onPress}
            disabled={isDisabled}
            className="rounded-md bg-primary"
            style={{ paddingVertical: 8, paddingHorizontal: 14, opacity: isDisabled ? 0.5 : 1 }}
        >
            <Text className="text-primary-foreground" style={{ fontSize: 13, fontWeight: '700' }}>
                {label}
            </Text>
        </Pressable>
    )
}
