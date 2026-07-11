import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { deriveUsername } from '@tinycld/core/lib/derive-username'
import { captureException } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { navigateToOrg } from '@tinycld/core/lib/org-url'
import { packageRegistry } from '@tinycld/core/lib/packages/static-registry'
import { pb as appPb, useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonIcon, ButtonText } from '@tinycld/core/ui/button'
import { Divider } from '@tinycld/core/ui/divider'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Building2, ChevronDown, ChevronRight, ExternalLink, Plus, X } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import type PocketBase from 'pocketbase'
import { useEffect, useState } from 'react'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'
import { PageHeader, RowIcon, SectionLabel, SlugTag } from './console-ui'

const mailInstalled = packageRegistry.some(p => p.slug === 'mail')

const setupSchema = z.object({
    orgName: z.string().min(3, 'Min 3 characters').max(45, 'Max 45 characters'),
    orgSlug: z
        .string()
        .min(3, 'Min 3 characters')
        .max(15, 'Max 15 characters')
        .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, 'Lowercase letters, numbers, and hyphens only'),
    ownerName: z.string().min(1, 'Name is required'),
    ownerUsername: z
        .string()
        .regex(
            /^[a-z0-9][a-z0-9_-]{0,31}$/,
            'Lowercase letters, numbers, _ or -, starting with a letter or number'
        ),
    email: z.email(),
    password: z.string().min(8, 'Min 8 characters'),
    mailDomain: mailInstalled
        ? z
              .string()
              .min(3, 'Min 3 characters')
              .max(253, 'Max 253 characters')
              .regex(
                  /^(?=.{1,253}$)(?!-)[a-z0-9-]{1,63}(?<!-)(\.(?!-)[a-z0-9-]{1,63}(?<!-))+$/,
                  'Enter a valid domain (e.g. example.com)'
              )
        : z.string().optional(),
})

const editOrgSchema = z.object({
    name: z.string().min(3, 'Min 3 characters').max(45, 'Max 45 characters'),
    ownerName: z.string().min(1, 'Name is required'),
    ownerEmail: z.email(),
    ownerPassword: z.union([z.string().min(8, 'Min 8 characters'), z.literal('')]),
})

function deriveSlug(name: string) {
    return name
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-|-$/g, '')
        .slice(0, 15)
}

interface OrgEntry {
    id: string
    name: string
    slug: string
    ownerEmail: string | null
    ownerId: string | null
    ownerName: string | null
    created: string
}

export function OrganizationsTab({ isVisible, pb }: { isVisible: boolean; pb: PocketBase }) {
    const [expandedOrgId, setExpandedOrgId] = useState<string | null>(null)
    const [showCreateForm, setShowCreateForm] = useState(false)

    // Live pbtsdb query. The orgs/user_org/users collections grant super-admins
    // cross-org read access (see the super_admin_rules migration), so the panel
    // reads them directly through the stores — no custom list endpoint. Each org's
    // owner is its role=owner user_org row's user; we left-join orgs → user_org →
    // users and select a flat row so the owner is part of the org row in one shaped
    // result. Joining the local `users` collection (rather than reading
    // `user_org.expand.user`) means an optimistically-created owner resolves
    // immediately, instead of reading "No owner assigned" until PB redelivered the
    // expanded record — a gap the edit form (its schema requires owner name/email)
    // then failed to save against.
    const [orgsCollection, userOrgCollection, usersCollection] = useStore(
        'orgs',
        'user_org',
        'users'
    )
    const { data: orgRows = [], isLoading: orgsLoading } = useLiveQuery(query => {
        // role=owner is filtered in a subquery, not in the join's `on` —
        // TanStack DB requires each join condition to be a single equality
        // expression, so the non-equality predicate can't sit alongside the
        // org↔user_org equality.
        const owners = query.from({ uo: userOrgCollection }).where(({ uo }) => eq(uo.role, 'owner'))
        return query
            .from({ orgs: orgsCollection })
            .join({ owner_uo: owners }, ({ orgs, owner_uo }) => eq(owner_uo.org, orgs.id), 'left')
            .join(
                { owner: usersCollection },
                ({ owner_uo, owner }) => eq(owner_uo.user, owner.id),
                'left'
            )
            .select(({ orgs, owner }) => ({
                id: orgs.id,
                name: orgs.name,
                slug: orgs.slug,
                created: orgs.created,
                ownerId: owner?.id,
                ownerName: owner?.name,
                ownerEmail: owner?.email,
            }))
    }, [])

    // `created` is unset on an optimistically-inserted row until the server
    // round-trips its autodate, so guard the comparison against undefined.
    const orgs: OrgEntry[] = [...orgRows]
        .sort((a, b) => (b.created ?? '').localeCompare(a.created ?? ''))
        .map(row => ({
            id: row.id,
            name: row.name,
            slug: row.slug,
            ownerEmail: row.ownerEmail ?? null,
            ownerId: row.ownerId ?? null,
            ownerName: row.ownerName ?? null,
            created: row.created,
        }))
    const isLoadingOrgs = orgsLoading

    if (!isVisible) return null

    const toggleExpanded = (orgId: string) => {
        setExpandedOrgId(prev => (prev === orgId ? null : orgId))
    }

    return (
        <View className="gap-6">
            <PageHeader
                title="Organizations"
                subtitle="Tenants on this deployment. Each org has one owner — create, edit, or impersonate them here."
                actions={
                    <Button
                        testID="org-new-toggle"
                        onPress={() => setShowCreateForm(v => !v)}
                        size="sm"
                        variant={showCreateForm ? 'outline' : 'default'}
                    >
                        <ButtonIcon as={showCreateForm ? X : Plus} />
                        <ButtonText>{showCreateForm ? 'Cancel' : 'New organization'}</ButtonText>
                    </Button>
                }
            />

            <CreateOrgSection
                isVisible={showCreateForm}
                onCreated={() => setShowCreateForm(false)}
            />

            <OrgList
                orgs={orgs}
                isLoading={isLoadingOrgs}
                expandedOrgId={expandedOrgId}
                onToggleExpanded={toggleExpanded}
                pb={pb}
            />
        </View>
    )
}

function OrgList({
    orgs,
    isLoading,
    expandedOrgId,
    onToggleExpanded,
    pb,
}: {
    orgs: OrgEntry[]
    isLoading: boolean
    expandedOrgId: string | null
    onToggleExpanded: (id: string) => void
    pb: PocketBase
}) {
    const mutedColor = useThemeColor('muted-foreground')

    if (isLoading) {
        return (
            <View className="p-5 items-center">
                <ActivityIndicator size="large" color={mutedColor} />
            </View>
        )
    }

    if (orgs.length === 0) {
        return (
            <View className="p-5 items-center rounded-xl border bg-surface-secondary border-border">
                <Text className="text-muted-foreground" style={{ fontSize: 16 }}>
                    No organizations yet. Create one to get started.
                </Text>
            </View>
        )
    }

    return (
        <View className="rounded-xl border overflow-hidden bg-surface-secondary border-border">
            {orgs.map((org, i) => (
                <View key={org.id}>
                    {i > 0 && <Divider />}
                    <OrgRow
                        org={org}
                        isExpanded={expandedOrgId === org.id}
                        onToggle={() => onToggleExpanded(org.id)}
                        pb={pb}
                    />
                </View>
            ))}
        </View>
    )
}

function OrgRow({
    org,
    isExpanded,
    onToggle,
    pb,
}: {
    org: OrgEntry
    isExpanded: boolean
    onToggle: () => void
    pb: PocketBase
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const warningColor = useThemeColor('warning')
    const hasOwner = org.ownerId !== null

    // Impersonation mints an auth token for the org owner — a superuser-level
    // action with no collection-rule equivalent, so it goes through the
    // requireAdmin-guarded endpoint (which a super-admin can reach) rather than
    // pb.collection('users').impersonate() (PB-superuser-only).
    const visit = async (e: { stopPropagation: () => void }) => {
        e.stopPropagation()
        if (!org.ownerId) return
        try {
            const { token, owner } = await pb.send<{
                token: string
                owner: { id: string; name: string; email: string }
            }>(`/api/admin/orgs/${org.id}/impersonate`, { method: 'POST' })
            appPb.authStore.save(token, { id: owner.id, email: owner.email } as never)
            // Navigate via the router (web AND native) — a raw window.location.href
            // assignment no-ops on native (RN has no window.location), so impersonate
            // silently went nowhere on device. navigateToOrg uses expo-router and
            // preserves the in-org subpath on web.
            navigateToOrg(org.slug)
        } catch (err) {
            captureException('setup.org.impersonate', err)
        }
    }

    return (
        <View testID={`org-row-${org.slug}`}>
            <Pressable
                testID={`org-row-toggle-${org.slug}`}
                onPress={onToggle}
                className="flex-row items-center gap-4 px-5 py-4"
            >
                <RowIcon Icon={Building2} />
                <View className="flex-1 gap-1">
                    <View className="flex-row gap-2 items-center">
                        <Text
                            className="text-foreground"
                            style={{ fontSize: 16, fontWeight: '600' }}
                        >
                            {org.name}
                        </Text>
                        <SlugTag>{org.slug}</SlugTag>
                    </View>
                    <Text style={{ fontSize: 13, color: hasOwner ? mutedColor : warningColor }}>
                        {org.ownerEmail ?? 'No owner assigned'}
                    </Text>
                </View>
                <View className="flex-row gap-2.5 items-center">
                    {hasOwner ? (
                        <Button
                            testID={`org-visit-${org.slug}`}
                            onPress={visit}
                            size="sm"
                            variant="ghost"
                        >
                            <ButtonText>Visit</ButtonText>
                            <ButtonIcon as={ExternalLink} />
                        </Button>
                    ) : null}
                    {isExpanded ? (
                        <ChevronDown size={18} color={mutedColor} />
                    ) : (
                        <ChevronRight size={18} color={mutedColor} />
                    )}
                </View>
            </Pressable>

            <OrgExpandedDetails isVisible={isExpanded} org={org} />
        </View>
    )
}

function OrgExpandedDetails({ isVisible, org }: { isVisible: boolean; org: OrgEntry }) {
    const [saveError, setSaveError] = useState<string | null>(null)
    const [orgsCollection, usersCollection] = useStore('orgs', 'users')

    const {
        control,
        handleSubmit,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        resolver: zodResolver(editOrgSchema),
        // `values` (not `defaultValues`) so the form re-syncs when the owner
        // relation resolves after mount — the row's edit form mounts as soon as
        // the org appears, which can be a render before the joined owner lands.
        // defaultValues snapshot once at mount and would lock in empty owner
        // fields, failing the schema's owner name/email requirement on save.
        values: {
            name: org.name,
            ownerName: org.ownerName ?? '',
            ownerEmail: org.ownerEmail ?? '',
            ownerPassword: '',
        },
        mode: 'onChange',
    })

    const save = useMutation({
        mutationFn: mutation(function* (data: z.infer<typeof editOrgSchema>) {
            yield orgsCollection.update(org.id, draft => {
                draft.name = data.name
            })
            if (org.ownerId) {
                yield usersCollection.update(org.ownerId, draft => {
                    draft.name = data.ownerName
                    draft.email = data.ownerEmail
                    if (data.ownerPassword) {
                        ;(draft as { password?: string }).password = data.ownerPassword
                    }
                })
            }
        }),
        onError: err => setSaveError(err instanceof Error ? err.message : 'Failed to update'),
    })

    const onSave = handleSubmit(data => {
        setSaveError(null)
        save.mutate(data)
    })

    if (!isVisible) return null

    const createdDate = org.created
        ? new Date(org.created).toLocaleDateString(undefined, {
              year: 'numeric',
              month: 'short',
              day: 'numeric',
          })
        : null

    const saveEnabled = isDirty && !save.isPending

    return (
        <View className="px-4 pb-4 gap-4">
            <Divider />

            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            {saveError && (
                <View className="rounded-lg p-2 bg-danger-soft">
                    <Text className="text-xs text-danger">{saveError}</Text>
                </View>
            )}

            <View className="gap-3">
                <SectionLabel>Organization</SectionLabel>

                <View className="flex-row gap-3 flex-wrap">
                    <View className="flex-1 min-w-[200px]">
                        <TextInput control={control} name="name" label="Name" />
                    </View>
                    <View className="flex-1 min-w-[200px] gap-1">
                        <Text
                            className="text-foreground"
                            style={{ fontSize: 14, fontWeight: '600' }}
                        >
                            Slug
                        </Text>
                        <Text
                            className="text-muted-foreground"
                            style={{ fontSize: 14, paddingVertical: 8 }}
                        >
                            {org.slug}
                        </Text>
                        {createdDate && (
                            <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                                Created {createdDate}
                            </Text>
                        )}
                    </View>
                </View>
            </View>

            <Divider />

            <View className="gap-3">
                <SectionLabel>Owner</SectionLabel>

                {org.ownerId ? (
                    <View className="flex-row gap-3 flex-wrap">
                        <View className="flex-1 min-w-[200px]">
                            <TextInput control={control} name="ownerName" label="Name" />
                        </View>
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="ownerEmail"
                                label="Email"
                                keyboardType="email-address"
                                autoCapitalize="none"
                                autoComplete="email"
                                textContentType="emailAddress"
                            />
                        </View>
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="ownerPassword"
                                label="New Password"
                                placeholder="Leave blank to keep current"
                                secureTextEntry
                                autoComplete="new-password"
                                textContentType="newPassword"
                            />
                        </View>
                    </View>
                ) : (
                    <Text className="text-muted-foreground" style={{ fontSize: 14 }}>
                        No owner assigned to this organization.
                    </Text>
                )}
            </View>

            <Button
                testID={`org-save-${org.slug}`}
                onPress={onSave}
                isDisabled={!saveEnabled}
                size="sm"
                className="self-start"
            >
                <ButtonText>{save.isPending ? 'Saving…' : 'Save changes'}</ButtonText>
            </Button>
        </View>
    )
}

function CreateOrgSection({ isVisible, onCreated }: { isVisible: boolean; onCreated: () => void }) {
    const [submitError, setSubmitError] = useState<string | null>(null)
    const [orgsCollection, usersCollection, userOrgCollection, orgProvisioningCollection] =
        useStore('orgs', 'users', 'user_org', 'org_provisioning')

    const {
        control,
        handleSubmit,
        setValue,
        watch,
        reset,
        formState: { errors, isSubmitted },
    } = useForm({
        resolver: zodResolver(setupSchema),
        defaultValues: {
            orgName: '',
            orgSlug: '',
            ownerName: '',
            ownerUsername: '',
            email: '',
            password: '',
            mailDomain: '',
        },
        mode: 'onChange',
    })

    const orgName = watch('orgName')
    const orgSlug = watch('orgSlug')
    const email = watch('email')
    const ownerUsername = watch('ownerUsername')

    // Auto-suggest the slug from the org name and the username from the email —
    // both stop auto-tracking once the admin edits them by hand.
    useEffect(() => {
        const prev = deriveSlug(orgName.slice(0, -1))
        if (!orgSlug || orgSlug === prev) {
            setValue('orgSlug', deriveSlug(orgName))
        }
    }, [orgName, orgSlug, setValue])

    useEffect(() => {
        const prev = deriveUsername(email.slice(0, -1))
        if (!ownerUsername || ownerUsername === prev) {
            setValue('ownerUsername', deriveUsername(email))
        }
    }, [email, ownerUsername, setValue])

    // The admin picks the owner username; a collision surfaces as a mutation
    // error they can fix and retry. pbtsdb yields run as one optimistic
    // transaction, so a mid-sequence failure rolls back the earlier inserts.
    const create = useMutation({
        mutationFn: mutation(function* (data: z.infer<typeof setupSchema>) {
            const ownerId = newRecordId()
            const orgId = newRecordId()
            yield usersCollection.insert({
                id: ownerId,
                username: data.ownerUsername,
                email: data.email,
                password: data.password,
                passwordConfirm: data.password,
                name: data.ownerName,
                emailVisibility: true,
                // Admin-created owners are pre-verified so they can sign in
                // immediately. Setting `verified` on an auth record requires the
                // users manageRule, which the super_admin_rules migration grants
                // to super-admins.
                verified: true,
            } as never)
            yield orgsCollection.insert({
                id: orgId,
                name: data.orgName,
                slug: data.orgSlug,
            } as never)
            yield userOrgCollection.insert({
                id: newRecordId(),
                user: ownerId,
                org: orgId,
                role: 'owner',
            } as never)
            // Don't write mail's collection from core — emit an org_provisioning
            // intent and let the mail package's server hook create the domain.
            if (mailInstalled && data.mailDomain) {
                yield orgProvisioningCollection.insert({
                    id: newRecordId(),
                    org: orgId,
                    mail_domain: data.mailDomain.trim().toLowerCase(),
                } as never)
            }
        }),
        onSuccess: () => {
            reset()
            onCreated()
        },
        onError: err => setSubmitError(err instanceof Error ? err.message : 'Failed to create org'),
    })

    const onSubmit = handleSubmit(data => {
        setSubmitError(null)
        create.mutate(data)
    })

    if (!isVisible) return null

    return (
        <View className="rounded-xl border overflow-hidden bg-surface-secondary border-border">
            <View className="p-5 gap-5">
                <Text className="text-foreground" style={{ fontSize: 15, fontWeight: '600' }}>
                    New organization &amp; owner
                </Text>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                {submitError && (
                    <View className="rounded-lg p-2 bg-danger-soft">
                        <Text className="text-xs text-danger">{submitError}</Text>
                    </View>
                )}

                <View className="gap-3">
                    <SectionLabel>Organization</SectionLabel>
                    <View className="flex-row gap-3 flex-wrap">
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="orgName"
                                label="Name"
                                placeholder="Acme Corp"
                            />
                        </View>
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="orgSlug"
                                label="Slug"
                                placeholder="acme-corp"
                                autoCapitalize="none"
                                hint="3-15 chars, lowercase, hyphens"
                            />
                        </View>
                    </View>
                </View>

                <Divider />

                <View className="gap-3">
                    <SectionLabel>Owner account</SectionLabel>
                    <View className="flex-row gap-3 flex-wrap">
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="ownerName"
                                label="Full name"
                                placeholder="Jane Smith"
                            />
                        </View>
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="email"
                                label="Email"
                                placeholder="owner@example.com"
                                keyboardType="email-address"
                                autoCapitalize="none"
                                autoComplete="email"
                                textContentType="emailAddress"
                            />
                        </View>
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="ownerUsername"
                                label="Username"
                                placeholder="jane"
                                autoCapitalize="none"
                                autoComplete="username"
                                textContentType="username"
                                hint="Must be unique — suggested from the email"
                            />
                        </View>
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="password"
                                label="Password"
                                placeholder="At least 8 characters"
                                secureTextEntry
                                autoComplete="new-password"
                                textContentType="newPassword"
                            />
                        </View>
                    </View>
                </View>

                {mailInstalled && (
                    <>
                        <Divider />
                        <View className="gap-3">
                            <SectionLabel>Mail</SectionLabel>
                            <View className="flex-row gap-3 flex-wrap">
                                <View className="flex-1 min-w-[200px]">
                                    <TextInput
                                        control={control}
                                        name="mailDomain"
                                        label="Mail domain"
                                        placeholder="example.com"
                                        autoCapitalize="none"
                                        hint="A personal mailbox is created under this domain for the owner"
                                    />
                                </View>
                            </View>
                        </View>
                    </>
                )}

                <Button
                    testID="org-create-submit"
                    onPress={onSubmit}
                    isDisabled={create.isPending}
                    size="sm"
                    className="self-start"
                >
                    <ButtonText>
                        {create.isPending ? 'Creating…' : 'Create organization'}
                    </ButtonText>
                </Button>
            </View>
        </View>
    )
}
