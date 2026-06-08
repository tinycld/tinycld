import { deriveUsername } from '@tinycld/core/lib/derive-username'
import { captureException } from '@tinycld/core/lib/errors'
import { packageRegistry } from '@tinycld/core/lib/packages/static-registry'
import { pb as appPb } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonIcon, ButtonText } from '@tinycld/core/ui/button'
import { Divider } from '@tinycld/core/ui/divider'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Building2, ChevronDown, ChevronRight, ExternalLink, Plus, X } from 'lucide-react-native'
import type PocketBase from 'pocketbase'
import { useCallback, useEffect, useState } from 'react'
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
    const [orgs, setOrgs] = useState<OrgEntry[]>([])
    const [isLoadingOrgs, setIsLoadingOrgs] = useState(true)
    const [expandedOrgId, setExpandedOrgId] = useState<string | null>(null)
    const [showCreateForm, setShowCreateForm] = useState(false)

    const fetchOrgs = useCallback(async () => {
        setIsLoadingOrgs(true)
        try {
            const orgRecords = await pb.collection('orgs').getFullList({ sort: '-created' })
            const entries: OrgEntry[] = await Promise.all(
                orgRecords.map(async org => {
                    let ownerEmail: string | null = null
                    let ownerId: string | null = null
                    let ownerName: string | null = null
                    try {
                        const userOrg = await pb
                            .collection('user_org')
                            .getFirstListItem(`org="${org.id}" && role="owner"`, {
                                expand: 'user',
                            })
                        const expanded = userOrg.expand as
                            | Record<string, { id?: string; email?: string; name?: string }>
                            | undefined
                        ownerEmail = expanded?.user?.email ?? null
                        ownerId = expanded?.user?.id ?? null
                        ownerName = expanded?.user?.name ?? null
                    } catch {
                        // no owner found
                    }
                    return {
                        id: org.id,
                        name: org.name as string,
                        slug: org.slug as string,
                        ownerEmail,
                        ownerId,
                        ownerName,
                        created: org.created as string,
                    }
                })
            )
            setOrgs(entries)
        } catch (err) {
            captureException('Failed to fetch orgs', err)
        } finally {
            setIsLoadingOrgs(false)
        }
    }, [pb])

    useEffect(() => {
        fetchOrgs()
    }, [fetchOrgs])

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
                pb={pb}
                onCreated={() => {
                    setShowCreateForm(false)
                    fetchOrgs()
                }}
            />

            <OrgList
                orgs={orgs}
                isLoading={isLoadingOrgs}
                expandedOrgId={expandedOrgId}
                onToggleExpanded={toggleExpanded}
                pb={pb}
                onUpdated={fetchOrgs}
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
    onUpdated,
}: {
    orgs: OrgEntry[]
    isLoading: boolean
    expandedOrgId: string | null
    onToggleExpanded: (id: string) => void
    pb: PocketBase
    onUpdated: () => void
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
                        onUpdated={onUpdated}
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
    onUpdated,
}: {
    org: OrgEntry
    isExpanded: boolean
    onToggle: () => void
    pb: PocketBase
    onUpdated: () => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const warningColor = useThemeColor('warning')
    const hasOwner = org.ownerId !== null

    const visit = async (e: { stopPropagation: () => void }) => {
        e.stopPropagation()
        if (!org.ownerId) return
        try {
            const impersonated = await pb.collection('users').impersonate(org.ownerId, 3600)
            appPb.authStore.save(impersonated.authStore.token, impersonated.authStore.record)
            if (typeof window !== 'undefined') {
                window.location.href = `/a/${org.slug}`
            }
        } catch (err) {
            captureException('Failed to impersonate user', err)
        }
    }

    return (
        <View>
            <Pressable onPress={onToggle} className="flex-row items-center gap-4 px-5 py-4">
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
                        <Button onPress={visit} size="sm" variant="ghost">
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

            <OrgExpandedDetails isVisible={isExpanded} org={org} pb={pb} onUpdated={onUpdated} />
        </View>
    )
}

function OrgExpandedDetails({
    isVisible,
    org,
    pb,
    onUpdated,
}: {
    isVisible: boolean
    org: OrgEntry
    pb: PocketBase
    onUpdated: () => void
}) {
    const [isSaving, setIsSaving] = useState(false)
    const [saveError, setSaveError] = useState<string | null>(null)

    const {
        control,
        handleSubmit,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        resolver: zodResolver(editOrgSchema),
        defaultValues: {
            name: org.name,
            ownerName: org.ownerName ?? '',
            ownerEmail: org.ownerEmail ?? '',
            ownerPassword: '',
        },
        mode: 'onChange',
    })

    const onSave = handleSubmit(async data => {
        setSaveError(null)
        setIsSaving(true)
        try {
            await pb.collection('orgs').update(org.id, { name: data.name })
            if (org.ownerId) {
                const userUpdate: Record<string, string> = {
                    name: data.ownerName,
                    email: data.ownerEmail,
                }
                if (data.ownerPassword) {
                    userUpdate.password = data.ownerPassword
                    userUpdate.passwordConfirm = data.ownerPassword
                }
                await pb.collection('users').update(org.ownerId, userUpdate)
            }
            onUpdated()
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : 'Failed to update')
        } finally {
            setIsSaving(false)
        }
    })

    if (!isVisible) return null

    const createdDate = org.created
        ? new Date(org.created).toLocaleDateString(undefined, {
              year: 'numeric',
              month: 'short',
              day: 'numeric',
          })
        : null

    const saveEnabled = isDirty && !isSaving

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
                            />
                        </View>
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="ownerPassword"
                                label="New Password"
                                placeholder="Leave blank to keep current"
                                secureTextEntry
                            />
                        </View>
                    </View>
                ) : (
                    <Text className="text-muted-foreground" style={{ fontSize: 14 }}>
                        No owner assigned to this organization.
                    </Text>
                )}
            </View>

            <Button onPress={onSave} isDisabled={!saveEnabled} size="sm" className="self-start">
                <ButtonText>{isSaving ? 'Saving…' : 'Save changes'}</ButtonText>
            </Button>
        </View>
    )
}

function CreateOrgSection({
    isVisible,
    pb,
    onCreated,
}: {
    isVisible: boolean
    pb: PocketBase
    onCreated: () => void
}) {
    const [submitError, setSubmitError] = useState<string | null>(null)
    const [isCreating, setIsCreating] = useState(false)

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
            email: '',
            password: '',
            mailDomain: '',
        },
        mode: 'onChange',
    })

    const orgName = watch('orgName')
    const orgSlug = watch('orgSlug')

    useEffect(() => {
        const prev = deriveSlug(orgName.slice(0, -1))
        if (!orgSlug || orgSlug === prev) {
            setValue('orgSlug', deriveSlug(orgName))
        }
    }, [orgName, orgSlug, setValue])

    const onSubmit = handleSubmit(async data => {
        setSubmitError(null)
        setIsCreating(true)

        let userId: string | null = null
        let orgId: string | null = null

        try {
            const base = deriveUsername(data.email)
            let user: { id: string } | null = null
            let lastErr: unknown = null
            for (let i = 0; i < 20; i++) {
                const candidate = i === 0 ? base : `${base}${i + 1}`
                try {
                    user = await pb.collection('users').create({
                        username: candidate,
                        email: data.email,
                        password: data.password,
                        passwordConfirm: data.password,
                        name: data.ownerName,
                        emailVisibility: true,
                        verified: true,
                    })
                    break
                } catch (err) {
                    lastErr = err
                    const validation = (err as { response?: { data?: Record<string, unknown> } })
                        ?.response?.data
                    const usernameErr = validation?.username as { code?: string } | undefined
                    if (usernameErr?.code === 'validation_not_unique') continue
                    throw err
                }
            }
            if (!user) throw lastErr ?? new Error('Failed to allocate a unique username')
            userId = user.id

            const org = await pb.collection('orgs').create({
                name: data.orgName,
                slug: data.orgSlug,
            })
            orgId = org.id

            if (mailInstalled && data.mailDomain) {
                await pb.collection('mail_domains').create({
                    org: orgId,
                    domain: data.mailDomain.trim().toLowerCase(),
                    verified: true,
                })
            }

            await pb.collection('user_org').create({
                user: userId,
                org: orgId,
                role: 'owner',
            })

            reset()
            onCreated()
        } catch (err) {
            const message = err instanceof Error ? err.message : 'Failed to create org'
            setSubmitError(message)

            if (orgId) {
                try {
                    await pb.collection('orgs').delete(orgId)
                } catch (cleanupErr) {
                    captureException('Cleanup failed: could not delete org', cleanupErr)
                }
            }
            if (userId) {
                try {
                    await pb.collection('users').delete(userId)
                } catch (cleanupErr) {
                    captureException('Cleanup failed: could not delete user', cleanupErr)
                }
            }
        } finally {
            setIsCreating(false)
        }
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
                            />
                        </View>
                        <View className="flex-1 min-w-[200px]">
                            <TextInput
                                control={control}
                                name="password"
                                label="Password"
                                placeholder="At least 8 characters"
                                secureTextEntry
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

                <Button onPress={onSubmit} isDisabled={isCreating} size="sm" className="self-start">
                    <ButtonText>{isCreating ? 'Creating…' : 'Create organization'}</ButtonText>
                </Button>
            </View>
        </View>
    )
}
