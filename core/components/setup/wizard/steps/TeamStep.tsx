import { useLiveQuery } from '@tanstack/react-db'
import { InviteLinkPanel } from '@tinycld/core/components/settings/members/InviteLinkPanel'
import { ROLE_LABELS } from '@tinycld/core/components/settings/members/types'
import {
    type InviteFormValues,
    type InviteResult,
    inviteSchema,
    useInviteMember,
} from '@tinycld/core/components/settings/members/use-invite-member'
import { SidebarSlot } from '@tinycld/core/components/sidebar-primitives/SidebarSlot'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { CORE_SLOT_TARGET } from '@tinycld/core/lib/setup/core-slots'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import {
    FormErrorSummary,
    SelectInput,
    TextInput,
    useForm,
    zodResolver,
} from '@tinycld/core/ui/form'
import { useState } from 'react'
import { Text, View } from 'react-native'

export function teamIsDone(userCount: number): boolean {
    return userCount > 1
}

export function useIsStepDone() {
    const [users] = useStore('users')
    const { data, isReady } = useLiveQuery(query =>
        query.from({ u: users }).select(({ u }) => ({ id: u.id }))
    )
    return isReady ? teamIsDone(data?.length ?? 0) : undefined
}

// Guests are for outside collaborators; the first team is members and admins.
const ROLE_OPTIONS = [
    { label: ROLE_LABELS.member, value: 'member' },
    { label: ROLE_LABELS.admin, value: 'admin' },
]

function useTeamInvite() {
    const form = useForm<InviteFormValues>({
        resolver: zodResolver(inviteSchema),
        defaultValues: { username: '', email: '', role: 'member' },
    })
    const [invited, setInvited] = useState<InviteResult | null>(null)
    const invite = useInviteMember({
        setError: form.setError,
        getValues: form.getValues,
        onInvited: result => {
            form.reset()
            setInvited(result)
        },
    })
    return {
        form,
        invited,
        onSubmit: form.handleSubmit(data => invite.mutate(data)),
        isPending: invite.isPending,
    }
}

function usePeople() {
    const [users] = useStore('users')
    const { data = [] } = useLiveQuery(query =>
        query
            .from({ u: users })
            .orderBy(({ u }) => u.name)
            .select(({ u }) => ({ id: u.id, name: u.name, username: u.username, role: u.role }))
    )
    return data.map(p => ({
        id: p.id,
        name: p.name || p.username,
        role: ROLE_LABELS[p.role] ?? p.role,
    }))
}

function InvitedLink({ invited }: { invited: InviteResult | null }) {
    if (!invited) return null
    return (
        <View testID="invite-link-step" className="mb-4 gap-2">
            <Text className="text-sm font-semibold text-foreground">
                Invite created. Share this link with your teammate.
            </Text>
            {/* Keyed so a second invite shows its own link, not the first one's. */}
            <InviteLinkPanel
                key={invited.userId}
                userId={invited.userId}
                initialUrl={invited.inviteUrl}
            />
        </View>
    )
}

function PersonRow({ name, role }: { name: string; role: string }) {
    return (
        <View className="flex-row items-center justify-between border-b border-border py-2">
            <Text className="text-sm text-foreground">{name}</Text>
            <Text className="text-xs text-muted-foreground">{role}</Text>
        </View>
    )
}

export default function TeamStep({ next }: SetupStepProps) {
    const { form, invited, onSubmit, isPending } = useTeamInvite()
    const { control, formState } = form
    const people = usePeople().map(p => <PersonRow key={p.id} name={p.name} role={p.role} />)
    return (
        <View className="max-w-[440px] gap-1">
            <Text className="text-2xl font-bold text-foreground">Your team</Text>
            <Text className="mb-3 text-sm text-muted-foreground">
                Invite the people who will use this workspace. Each invite makes a link that lets
                them choose a password.
            </Text>
            <SidebarSlot target={CORE_SLOT_TARGET} slot="setup-team" />
            <FormErrorSummary errors={formState.errors} isEnabled={formState.isSubmitted} />
            <TextInput
                control={control}
                name="username"
                label="Username"
                placeholder="alice"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput
                control={control}
                name="email"
                label="Email (optional)"
                hint="Their existing address, so you can send the invite link to them."
                placeholder="alice@company.com"
                autoCapitalize="none"
                autoComplete="email"
                keyboardType="email-address"
            />
            <SelectInput
                control={control}
                name="role"
                label="Role"
                options={ROLE_OPTIONS}
                horizontal
            />
            <Button
                variant="outline"
                className="mb-4 self-start"
                onPress={onSubmit}
                isDisabled={isPending}
            >
                <ButtonText>Send invite</ButtonText>
            </Button>
            <InvitedLink invited={invited} />
            <Text className="text-sm font-semibold text-foreground">People in this workspace</Text>
            <View className="mb-4">{people}</View>
            <Button className="self-start" onPress={next}>
                <ButtonText>Continue</ButtonText>
            </Button>
        </View>
    )
}
