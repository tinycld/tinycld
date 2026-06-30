import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { packageSystemSettings } from '@tinycld/core/lib/packages/derive-components'
import { packageRegistry } from '@tinycld/core/lib/packages/static-registry'
import { pb as appPb } from '@tinycld/core/lib/pocketbase'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import {
    FormErrorSummary,
    SelectInput,
    TextInput,
    Toggle,
    useForm,
    z,
    zodResolver,
} from '@tinycld/core/ui/form'
import { type ReactNode, Suspense, useState } from 'react'
import type { Control, FieldValues, Path } from 'react-hook-form'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'
import { PageHeader, SectionLabel } from './console-ui'
import {
    deliveryEnabledValue,
    fromAddressSchema,
    isDeliveryEnabled,
    type SettingRow,
    sentryDsnSchema,
    shouldPersistSecret,
    vapidSubjectSchema,
} from './system-settings-logic'
import { useSystemSettings } from './system-settings-store'

export function SettingsTab({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="gap-6">
            <PageHeader
                title="Settings"
                subtitle="System-wide configuration for this deployment. These apply to the entire system, not a single organization."
            />
            <SentrySettings />
            <VapidSettings />
            <MailSettings />
            <PackageSettings />
        </View>
    )
}

function Panel({ label, children }: { label: string; children: ReactNode }) {
    return (
        <View className="gap-4 p-5 rounded-2xl bg-surface-secondary border border-border">
            <SectionLabel>{label}</SectionLabel>
            {children}
        </View>
    )
}

const sentrySchema = z.object({ dsn: sentryDsnSchema })

function SentrySettings() {
    const { byKey, upsert } = useSystemSettings()
    const existing = byKey.get('sentry.dsn')

    const {
        control,
        handleSubmit,
        setError,
        formState: { errors, isSubmitting, isSubmitted, isDirty },
    } = useForm({
        resolver: zodResolver(sentrySchema),
        // `values` (not defaultValues) so the field reactively re-syncs when the
        // stored DSN loads async or changes server-side. Avoids a useEffect+reset.
        values: { dsn: existing?.value ?? '' },
        mode: 'onChange',
    })

    const onSubmit = handleSubmit(data =>
        upsert.mutate(
            { key: 'sentry.dsn', value: data.dsn, isSecret: false },
            {
                onError: err =>
                    setError('dsn', {
                        message: err instanceof Error ? err.message : 'Failed to save',
                    }),
            }
        )
    )

    return (
        <Panel label="Error reporting (Sentry)">
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                The DSN errors are reported to. Leave blank to disable. Web clients pick up a change
                on their next load; native apps on their next build.
            </Text>
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <TextInput
                control={control}
                name="dsn"
                label="Sentry DSN"
                placeholder="https://…@…ingest.sentry.io/…"
                autoCapitalize="none"
                hint="Public value — safe to expose in the web client."
            />
            <SaveRow
                testID="sentry-dsn-save"
                onPress={onSubmit}
                isPending={upsert.isPending}
                isDisabled={isSubmitting || !isDirty}
            />
        </Panel>
    )
}

const vapidSchema = z.object({
    publicKey: z.string(),
    // A secret field: blank means "leave the stored value unchanged" (write-only).
    privateKey: z.string(),
    subject: vapidSubjectSchema,
})

// The operator never needs to READ the VAPID keys: the server signs with the
// private key and the browser receives the public key through the push-subscribe
// flow. So the common path is one click — generate AND persist server-side, with
// the private key never reaching the client. Pasting an existing pair (migration
// from another deployment) stays available behind a disclosure.
function VapidSettings() {
    const { byKey, upsert } = useSystemSettings()
    const publicKey = byKey.get('vapid.public_key')
    const privateKey = byKey.get('vapid.private_key')
    const subject = byKey.get('vapid.subject')
    const configured = Boolean(publicKey?.value && privateKey?.value)

    const [generateError, setGenerateError] = useState<string | null>(null)
    const [isGenerating, setIsGenerating] = useState(false)
    const [showPaste, setShowPaste] = useState(false)

    // Generate + persist in one action. The endpoint writes both keys server-side
    // and returns only the public key; the live query refreshes the status below.
    // Replacing the keys invalidates existing browser push subscriptions, so this
    // is a deliberate action.
    const generate = async () => {
        setGenerateError(null)
        setIsGenerating(true)
        try {
            const res = await fetch(`${PB_SERVER_ADDR}/api/admin/vapid/generate`, {
                method: 'POST',
                headers: { Authorization: appPb.authStore.token },
            })
            if (!res.ok) {
                const data = (await res.json().catch(() => ({}))) as { error?: string }
                throw new Error(data.error ?? 'Failed to generate keys')
            }
        } catch (err) {
            setGenerateError(err instanceof Error ? err.message : 'Failed to generate keys')
        } finally {
            setIsGenerating(false)
        }
    }

    return (
        <Panel label="Web push (VAPID)">
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                The keypair browser push notifications are signed with. Generating replaces any
                existing keys and invalidates current push subscriptions.
            </Text>
            <View className="flex-row items-center gap-2">
                <Text
                    style={{ fontSize: 13 }}
                    className={configured ? 'text-success' : 'text-muted-foreground'}
                >
                    {configured ? 'Configured ✓' : 'Not configured'}
                </Text>
            </View>
            {generateError && (
                <View className="rounded-lg p-2 bg-danger-soft">
                    <Text className="text-xs text-danger">{generateError}</Text>
                </View>
            )}
            <View className="flex-row">
                <Button
                    testID="vapid-generate"
                    onPress={generate}
                    size="sm"
                    isDisabled={isGenerating}
                >
                    <ButtonText>
                        {isGenerating
                            ? 'Generating…'
                            : configured
                              ? 'Regenerate keypair'
                              : 'Generate keypair'}
                    </ButtonText>
                </Button>
            </View>

            <Pressable onPress={() => setShowPaste(v => !v)} accessibilityRole="button">
                <Text className="text-muted-foreground" style={{ fontSize: 12.5 }}>
                    {showPaste ? '▾' : '▸'} Use existing keys (migrate from another deployment)
                </Text>
            </Pressable>
            <VapidPaste
                isVisible={showPaste}
                upsert={upsert}
                privateKey={privateKey}
                subject={subject}
            />
        </Panel>
    )
}

// VapidPaste is the bring-your-own escape hatch: paste a pre-existing keypair
// (e.g. when migrating off the old env-var deployment so existing push
// subscriptions keep working). Same write-only treatment for the private key.
function VapidPaste({
    isVisible,
    upsert,
    privateKey,
    subject,
}: {
    isVisible: boolean
    upsert: ReturnType<typeof useSystemSettings>['upsert']
    privateKey: SettingRow | undefined
    subject: SettingRow | undefined
}) {
    const {
        control,
        handleSubmit,
        formState: { errors, isSubmitting, isSubmitted },
    } = useForm({
        resolver: zodResolver(vapidSchema),
        values: { publicKey: '', privateKey: '', subject: subject?.value ?? '' },
        mode: 'onChange',
    })

    const onSubmit = handleSubmit(async data => {
        if (data.publicKey.trim() !== '') {
            await upsert.mutateAsync({
                key: 'vapid.public_key',
                value: data.publicKey,
                isSecret: false,
            })
        }
        if (shouldPersistSecret(data.privateKey)) {
            await upsert.mutateAsync({
                key: 'vapid.private_key',
                value: data.privateKey,
                isSecret: true,
            })
        }
        await upsert.mutateAsync({ key: 'vapid.subject', value: data.subject, isSecret: false })
    })

    if (!isVisible) return null

    return (
        <View className="gap-3 pt-1">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <TextInput
                control={control}
                name="publicKey"
                label="Public key"
                autoCapitalize="none"
                hint="Leave blank to keep the current public key."
            />
            <SecretField
                control={control}
                name="privateKey"
                label="Private key"
                existing={privateKey}
            />
            <TextInput
                control={control}
                name="subject"
                label="Subject"
                placeholder="mailto:admin@example.com"
                autoCapitalize="none"
            />
            <SaveRow
                testID="vapid-save"
                onPress={onSubmit}
                isPending={upsert.isPending}
                isDisabled={isSubmitting}
            />
        </View>
    )
}

// SecretField renders a write-only input for a secret system setting. It NEVER
// seeds the stored value into the field (the value would otherwise round-trip
// through the DOM); instead it shows whether a value is configured and a hint
// that leaving it blank keeps the current one. Submitting blank means "unchanged"
// — the caller must skip the write when the field is empty.
// Core transactional mail (invites, password reset, share notifications). These
// are the SHARED `mail.*` keys the core mailer reads. When the mail feature
// package is installed it owns provider selection + the server token via its own
// "Provider" panel, so here we only surface the from-address and the delivery
// switch to avoid two editors of the same key. In a mail-less assembly we also
// surface provider + token so core mail is configurable on its own.
const mailProviders = [
    { label: 'Postmark', value: 'postmark' },
    { label: 'Self-hosted SMTP', value: 'smtp' },
]

const mailSchema = z.object({
    provider: z.enum(['postmark', 'smtp']),
    serverToken: z.string(), // secret, write-only
    fromAddress: fromAddressSchema,
    deliveryEnabled: z.boolean(),
})

function MailSettings() {
    const { byKey, upsert } = useSystemSettings()
    // Build-time registry (same source that decides whether mail's Provider
    // panel renders), so the two panels' key ownership stays consistent.
    const mailPackageInstalled = packageRegistry.some(p => p.slug === 'mail')

    const provider = byKey.get('mail.provider')
    const serverToken = byKey.get('mail.postmark_server_token')
    const fromAddress = byKey.get('mail.from_address')
    const deliveryEnabled = byKey.get('mail.delivery_enabled')

    const {
        control,
        handleSubmit,
        setError,
        formState: { errors, isSubmitting, isSubmitted, isDirty },
    } = useForm({
        resolver: zodResolver(mailSchema),
        values: {
            provider: (provider?.value || 'postmark') as 'postmark' | 'smtp',
            serverToken: '',
            fromAddress: fromAddress?.value ?? '',
            deliveryEnabled: isDeliveryEnabled(deliveryEnabled?.value),
        },
        mode: 'onChange',
    })

    const onSubmit = handleSubmit(async data => {
        try {
            await upsert.mutateAsync({
                key: 'mail.from_address',
                value: data.fromAddress,
                isSecret: false,
            })
            await upsert.mutateAsync({
                key: 'mail.delivery_enabled',
                value: deliveryEnabledValue(data.deliveryEnabled),
                isSecret: false,
            })
            if (!mailPackageInstalled) {
                await upsert.mutateAsync({
                    key: 'mail.provider',
                    value: data.provider,
                    isSecret: false,
                })
                if (shouldPersistSecret(data.serverToken)) {
                    await upsert.mutateAsync({
                        key: 'mail.postmark_server_token',
                        value: data.serverToken,
                        isSecret: true,
                    })
                }
            }
        } catch (err) {
            setError('fromAddress', {
                message: err instanceof Error ? err.message : 'Failed to save',
            })
        }
    })

    return (
        <Panel label="Mail — Sending (transactional)">
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                Outbound mail for invites, password resets, and share notifications.
                {mailPackageInstalled
                    ? ' The provider and credentials are configured in the Mail package’s Provider panel below.'
                    : ' Choose a provider and credentials for delivery.'}
            </Text>
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            {!mailPackageInstalled && (
                <>
                    <SelectInput
                        control={control}
                        name="provider"
                        label="Provider"
                        options={mailProviders}
                    />
                    <SecretField
                        control={control}
                        name="serverToken"
                        label="Postmark server token"
                        existing={serverToken}
                    />
                </>
            )}

            <TextInput
                control={control}
                name="fromAddress"
                label="From address"
                placeholder="noreply@tinycld.org"
                autoCapitalize="none"
                hint="The default From for transactional mail. Blank uses noreply@tinycld.org."
            />
            <Toggle
                control={control}
                name="deliveryEnabled"
                label="Deliver mail"
                hint="Off logs emails instead of sending them. Dev/test builds always log."
            />
            <SaveRow
                testID="mail-settings-save"
                onPress={onSubmit}
                isPending={upsert.isPending}
                isDisabled={isSubmitting || !isDirty}
            />
        </Panel>
    )
}

function SecretField<T extends FieldValues>({
    control,
    name,
    label,
    existing,
}: {
    control: Control<T>
    name: Path<T>
    label: string
    existing: SettingRow | undefined
}) {
    const configured = Boolean(existing?.value)
    return (
        <View className="gap-1.5">
            <TextInput
                control={control}
                name={name}
                label={label}
                placeholder={configured ? '•••••••• (configured)' : 'Not set'}
                secureTextEntry
                autoCapitalize="none"
                hint={
                    configured
                        ? 'Configured. Enter a new value to replace it; leave blank to keep it.'
                        : 'Not set.'
                }
            />
        </View>
    )
}

function SaveRow({
    testID,
    onPress,
    isPending,
    isDisabled,
}: {
    testID: string
    onPress: () => void
    isPending: boolean
    isDisabled: boolean
}) {
    return (
        <View className="flex-row justify-end">
            <Button testID={testID} onPress={onPress} size="sm" isDisabled={isDisabled}>
                <ButtonText>{isPending ? 'Saving…' : 'Save'}</ButtonText>
            </Button>
        </View>
    )
}

// Panels contributed by installed packages via their manifest `systemSettings`
// (e.g. mail's provider credentials). Each is a lazy component; render it in
// Suspense, grouped under its package name. Empty when no package contributes one.
function PackageSettings() {
    if (packageSystemSettings.length === 0) return null
    return (
        <>
            {packageSystemSettings.map(group =>
                group.panels.map(panel => {
                    const PanelComponent = panel.Component
                    return (
                        <Panel
                            key={`${group.pkgSlug}:${panel.slug}`}
                            label={`${group.packageName} — ${panel.label}`}
                        >
                            <Suspense
                                fallback={
                                    <View className="py-6 items-center">
                                        <ActivityIndicator />
                                    </View>
                                }
                            >
                                <PanelComponent />
                            </Suspense>
                        </Panel>
                    )
                })
            )}
        </>
    )
}
