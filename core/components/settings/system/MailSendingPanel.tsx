import { usePackage } from '@tinycld/core/lib/packages/use-packages'
import {
    FormErrorSummary,
    SelectInput,
    TextInput,
    Toggle,
    useForm,
    z,
    zodResolver,
} from '@tinycld/core/ui/form'
import type { Control } from 'react-hook-form'
import {
    deliveryEnabledValue,
    fromAddressSchema,
    isDeliveryEnabled,
    type SettingRow,
    shouldPersistSecret,
} from '../../setup/system-settings-logic'
import { useSystemSettings } from '../../setup/system-settings-store'
import { Panel, PanelIntro, SaveRow, SecretField } from './panel-chrome'

// Core transactional mail (invites, password reset, share notifications). These
// are the SHARED `mail.*` keys the core mailer reads. When the mail feature
// package is installed it owns provider selection + the server token via its own
// system-settings panel, so here we only surface the from-address and the
// delivery switch to avoid two editors of the same key. In a mail-less assembly
// we also surface provider + token so core mail is configurable on its own.
//
// Note the asymmetry: `mail.from_address` and `mail.delivery_enabled` are owned
// HERE in every assembly — the mail package's panel never writes them.
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

export function MailSendingPanel() {
    const { byKey, upsert } = useSystemSettings()
    // Runtime registry, so a DB-installed mail package suppresses these fields
    // just as a bundled one does. Core owns the `mail.*` keys either way (the
    // transactional mailer reads them); this only decides whether the mail
    // package's own panel is the editor for provider + token, so that one key
    // never has two editors.
    const mailPackageInstalled = usePackage('mail') !== null

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
            <PanelIntro>
                Outbound mail for invites, password resets, and share notifications.
                {mailPackageInstalled
                    ? ' The provider and credentials are configured in the Mail package’s own system panel.'
                    : ' Choose a provider and credentials for delivery.'}
            </PanelIntro>
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            <ProviderFields
                isVisible={!mailPackageInstalled}
                control={control}
                serverToken={serverToken}
            />

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

// Shown only in a mail-less assembly, where nothing else edits these two keys.
function ProviderFields({
    isVisible,
    control,
    serverToken,
}: {
    isVisible: boolean
    control: Control<z.infer<typeof mailSchema>>
    serverToken: SettingRow | undefined
}) {
    if (!isVisible) return null
    return (
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
    )
}
