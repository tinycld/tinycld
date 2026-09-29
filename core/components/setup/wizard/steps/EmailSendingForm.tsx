import {
    deliveryEnabledValue,
    fromAddressSchema,
    isDeliveryEnabled,
    type SettingRow,
    shouldPersistSecret,
} from '@tinycld/core/components/setup/system-settings-logic'
import { useSystemSettings } from '@tinycld/core/components/setup/system-settings-store'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import {
    type Control,
    Controller,
    FormErrorSummary,
    TextInput,
    Toggle,
    useForm,
    z,
    zodResolver,
} from '@tinycld/core/ui/form'
import { useWatch } from 'react-hook-form'
import { Text, View } from 'react-native'
import { SetupContinueButton } from '../SetupContinueButton'
import { SetupTabs } from '../SetupTabs'

// The keys core's own mailer reads (core/server/mailer). A package that sends
// mail reads the same ones, and keeps any provider option only it needs (DKIM,
// inbound mode) in its own Settings → System panel. Postmark needs one token
// here, the account token: the server derives the sending token from it,
// creating the Postmark server the first time anything asks for that token.
const KEYS = {
    deliveryEnabled: 'mail.delivery_enabled',
    fromAddress: 'mail.from_address',
    provider: 'mail.provider',
    postmarkAccountToken: 'mail.postmark_account_token',
    smtpPublicHostname: 'mail.smtp_public_hostname',
} as const

const PROVIDER_TABS = [
    { value: 'postmark', label: 'Postmark' },
    { value: 'smtp', label: 'Self-hosted SMTP' },
] as const

type Provider = (typeof PROVIDER_TABS)[number]['value']

export interface StoredEmailSettings {
    deliveryEnabled: boolean
    fromAddress: string
    provider: Provider
    hasAccountToken: boolean
    smtpPublicHostname: string
}

export function storedEmailSettingsOf(byKey: Map<string, SettingRow>): StoredEmailSettings {
    const value = (key: string) => byKey.get(key)?.value ?? ''
    return {
        deliveryEnabled: isDeliveryEnabled(byKey.get(KEYS.deliveryEnabled)?.value),
        fromAddress: value(KEYS.fromAddress),
        provider: value(KEYS.provider) === 'smtp' ? 'smtp' : 'postmark',
        hasAccountToken: value(KEYS.postmarkAccountToken) !== '',
        smtpPublicHostname: value(KEYS.smtpPublicHostname),
    }
}

/**
 * Secrets are write-only: blank means "keep what is stored". The account token
 * is required to send through Postmark, so it is asked for only when none is
 * stored yet. Nothing about the provider is checked while delivery is off.
 */
export function emailSchemaFor(stored: { hasAccountToken: boolean }) {
    return z
        .object({
            deliveryEnabled: z.boolean(),
            fromAddress: fromAddressSchema,
            provider: z.enum(['postmark', 'smtp']),
            postmarkAccountToken: z.string(),
            smtpPublicHostname: z.string().trim(),
        })
        .superRefine((data, ctx) => {
            if (!data.deliveryEnabled) return
            if (
                data.provider === 'postmark' &&
                !stored.hasAccountToken &&
                !shouldPersistSecret(data.postmarkAccountToken)
            ) {
                ctx.addIssue({
                    code: 'custom',
                    path: ['postmarkAccountToken'],
                    message: 'Enter your Postmark account token',
                })
            }
            if (data.provider === 'smtp' && data.smtpPublicHostname === '') {
                ctx.addIssue({
                    code: 'custom',
                    path: ['smtpPublicHostname'],
                    message: 'Enter the hostname this server sends mail as',
                })
            }
        })
}

export type EmailForm = z.infer<ReturnType<typeof emailSchemaFor>>

export interface SettingWrite {
    key: string
    value: string
    isSecret: boolean
}

/**
 * Every write one Continue makes, in order. With delivery off only the switch
 * and the From address are stored: the provider fields are hidden then, so
 * whatever they held is not what the person reviewed.
 */
export function emailWritesOf(data: EmailForm): SettingWrite[] {
    const writes: SettingWrite[] = [
        {
            key: KEYS.deliveryEnabled,
            value: deliveryEnabledValue(data.deliveryEnabled),
            isSecret: false,
        },
        { key: KEYS.fromAddress, value: data.fromAddress, isSecret: false },
    ]
    if (!data.deliveryEnabled) return writes
    writes.push({ key: KEYS.provider, value: data.provider, isSecret: false })
    if (data.provider === 'postmark') {
        if (shouldPersistSecret(data.postmarkAccountToken)) {
            writes.push({
                key: KEYS.postmarkAccountToken,
                value: data.postmarkAccountToken,
                isSecret: true,
            })
        }
        return writes
    }
    writes.push({ key: KEYS.smtpPublicHostname, value: data.smtpPublicHostname, isSecret: false })
    return writes
}

function useEmailSendingForm(next: () => void, stored: StoredEmailSettings) {
    const { upsert } = useSystemSettings('mail')
    const form = useForm<EmailForm>({
        resolver: zodResolver(emailSchemaFor(stored)),
        defaultValues: {
            deliveryEnabled: stored.deliveryEnabled,
            fromAddress: stored.fromAddress,
            provider: stored.provider,
            postmarkAccountToken: '',
            smtpPublicHostname: stored.smtpPublicHostname,
        },
    })
    const save = useMutation({
        mutationFn: async (data: EmailForm) => {
            // In order, one row at a time: the store's upsert reads the row map
            // it holds, and a parallel batch could insert a key twice.
            for (const write of emailWritesOf(data)) {
                await upsert.mutateAsync(write)
            }
        },
        onSuccess: next,
        onError: handleMutationErrorsWithForm({
            setError: form.setError,
            getValues: form.getValues,
            operation: 'setup.email',
        }),
    })
    return {
        form,
        onSubmit: form.handleSubmit(data => save.mutate(data)),
        isPending: save.isPending,
    }
}

function secretHint(isSet: boolean, whenUnset: string): string {
    return isSet ? 'Configured. Leave blank to keep it.' : whenUnset
}

function PostmarkFields({
    control,
    stored,
    isVisible,
}: {
    control: Control<EmailForm>
    stored: StoredEmailSettings
    isVisible: boolean
}) {
    if (!isVisible) return null
    return (
        <TextInput
            control={control}
            name="postmarkAccountToken"
            label="Account token"
            testID="setup-email-account-token"
            secureTextEntry
            autoCapitalize="none"
            autoCorrect={false}
            hint={secretHint(
                stored.hasAccountToken,
                'From Account → API Tokens in Postmark. The server uses it to create its own Postmark server and to add your email domains.'
            )}
        />
    )
}

function SmtpFields({ control, isVisible }: { control: Control<EmailForm>; isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <TextInput
            control={control}
            name="smtpPublicHostname"
            label="Public hostname"
            testID="setup-email-smtp-hostname"
            placeholder="mail.example.com"
            autoCapitalize="none"
            autoCorrect={false}
            hint="The hostname this server sends mail as. DKIM and inbound options are in Settings → System."
        />
    )
}

function ProviderSection({
    control,
    stored,
    isVisible,
}: {
    control: Control<EmailForm>
    stored: StoredEmailSettings
    isVisible: boolean
}) {
    const provider = useWatch({ control, name: 'provider' })
    if (!isVisible) return null
    return (
        <View>
            <TextInput
                control={control}
                name="fromAddress"
                label="From address"
                testID="setup-email-from"
                placeholder="noreply@tinycld.org"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="email-address"
                hint="The sender of invites and notifications. Blank uses noreply@tinycld.org."
            />
            <Text className="mb-2 text-sm font-semibold text-foreground">Provider</Text>
            <Controller
                control={control}
                name="provider"
                render={({ field }) => (
                    <SetupTabs
                        tabs={PROVIDER_TABS}
                        value={field.value}
                        onChange={field.onChange}
                        testID="setup-email-provider"
                    />
                )}
            />
            <PostmarkFields control={control} stored={stored} isVisible={provider === 'postmark'} />
            <SmtpFields control={control} isVisible={provider === 'smtp'} />
        </View>
    )
}

function LoadedEmailSendingForm({
    next,
    stored,
}: {
    next: () => void
    stored: StoredEmailSettings
}) {
    const { form, onSubmit, isPending } = useEmailSendingForm(next, stored)
    const { control, formState } = form
    const deliveryEnabled = useWatch({ control, name: 'deliveryEnabled' })
    return (
        <View>
            <FormErrorSummary errors={formState.errors} isEnabled={formState.isSubmitted} />
            <Toggle
                control={control}
                name="deliveryEnabled"
                label="Deliver mail"
                hint="Off writes each email to the server log instead of sending it."
            />
            <ProviderSection control={control} stored={stored} isVisible={deliveryEnabled} />
            <SetupContinueButton onPress={onSubmit} isDisabled={isPending} />
        </View>
    )
}

/** One form for every mail setting the step touches; Continue saves them all. */
export function EmailSendingForm({ next }: { next: () => void }) {
    const { byKey, isReady } = useSystemSettings('mail')
    // The stored values are the form's defaults, so the form mounts only once
    // they have loaded; an empty first render would seed blanks.
    if (!isReady) return null
    return <LoadedEmailSendingForm next={next} stored={storedEmailSettingsOf(byKey)} />
}
