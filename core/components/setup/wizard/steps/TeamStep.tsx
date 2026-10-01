import { useLiveQuery } from '@tanstack/react-db'
import { InviteLinkPanel } from '@tinycld/core/components/settings/members/InviteLinkPanel'
import { ROLE_DESCRIPTIONS, ROLE_LABELS } from '@tinycld/core/components/settings/members/types'
import {
    type InviteFormValues,
    type InviteResult,
    inviteSchema,
    useInviteMember,
} from '@tinycld/core/components/settings/members/use-invite-member'
import { SetupContinueButton } from '@tinycld/core/components/setup/wizard/SetupContinueButton'
import { StepHeading } from '@tinycld/core/components/setup/wizard/StepHeading'
import { SidebarSlot } from '@tinycld/core/components/sidebar-primitives/SidebarSlot'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { CORE_SLOT_TARGET } from '@tinycld/core/lib/setup/core-slots'
import {
    SETUP_INVITE_EMAIL_TEST_ID,
    SETUP_INVITE_SEND_TEST_ID,
    SETUP_INVITE_USERNAME_TEST_ID,
} from '@tinycld/core/lib/setup/step-ids'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import {
    FormErrorSummary,
    RadioInput,
    TextInput,
    useForm,
    zodResolver,
} from '@tinycld/core/ui/form'
import { useState } from 'react'
import { Text, View } from 'react-native'
import { initialsOf } from '../use-workspace-summary'

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
    { label: ROLE_LABELS.member, value: 'member', description: ROLE_DESCRIPTIONS.member },
    { label: ROLE_LABELS.admin, value: 'admin', description: ROLE_DESCRIPTIONS.admin },
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
        errorsOnForm: true,
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
        initials: initialsOf(p.name || p.username),
        role: ROLE_LABELS[p.role] ?? p.role,
    }))
}

function InvitedLink({ invited }: { invited: InviteResult | null }) {
    if (!invited) return null
    return (
        <View
            testID="invite-link-step"
            className="mb-5 gap-2 rounded-xl border border-primary/40 bg-accent p-4"
        >
            <Text className="text-sm font-semibold text-foreground">Invite created.</Text>
            {/* Keyed so a second invite shows its own link, not the first one's. */}
            <InviteLinkPanel
                key={invited.userId}
                userId={invited.userId}
                initialUrl={invited.inviteUrl}
                emailedTo={invited.emailedTo}
            />
        </View>
    )
}

function PersonRow({ name, initials, role }: { name: string; initials: string; role: string }) {
    return (
        <View className="flex-row items-center gap-3 py-2.5">
            <View className="size-8 items-center justify-center rounded-full bg-secondary">
                <Text className="text-[11px] font-bold text-foreground">{initials}</Text>
            </View>
            <Text className="flex-1 text-sm text-foreground">{name}</Text>
            <View className="rounded-full border border-border px-2.5 py-0.5">
                <Text className="text-[11px] font-medium text-muted-foreground">{role}</Text>
            </View>
        </View>
    )
}

export default function TeamStep({ next }: SetupStepProps) {
    const { form, invited, onSubmit, isPending } = useTeamInvite()
    const { control, formState } = form
    const people = usePeople()
    const rows = people.map(p => (
        <PersonRow key={p.id} name={p.name} initials={p.initials} role={p.role} />
    ))
    // The owner is always listed; Continue waits for a second person so a
    // press meant for "Add user and invite" cannot skip past the step.
    const hasTeammate = people.length > 1
    return (
        <View>
            <StepHeading
                title="Your team"
                lead="Invite the people who will join this organization. Each invite makes a link that lets them choose a password."
            />
            <SidebarSlot target={CORE_SLOT_TARGET} slot="setup-team" />
            <FormErrorSummary errors={formState.errors} isEnabled={formState.isSubmitted} />
            <TextInput
                control={control}
                name="username"
                label="Username"
                testID={SETUP_INVITE_USERNAME_TEST_ID}
                placeholder="alice"
                autoCapitalize="none"
                autoCorrect={false}
            />
            <TextInput
                control={control}
                name="email"
                label="Email (optional)"
                testID={SETUP_INVITE_EMAIL_TEST_ID}
                hint="Their existing address. We email the invite link here."
                placeholder="alice@company.com"
                autoCapitalize="none"
                autoComplete="email"
                keyboardType="email-address"
            />
            <RadioInput control={control} name="role" label="Role" options={ROLE_OPTIONS} />
            <Button
                variant="outline"
                className="mb-6 self-start"
                onPress={onSubmit}
                isDisabled={isPending}
                testID={SETUP_INVITE_SEND_TEST_ID}
            >
                <ButtonText>Add user and invite</ButtonText>
            </Button>
            <InvitedLink invited={invited} />
            <Text className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                People in this organization
            </Text>
            <View className="mb-6 mt-1">{rows}</View>
            <SetupContinueButton onPress={next} isDisabled={!hasTeammate} />
        </View>
    )
}
