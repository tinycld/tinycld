import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { pb as appPb } from '@tinycld/core/lib/pocketbase'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import {
    type SettingRow,
    shouldPersistSecret,
    vapidSubjectSchema,
} from '../../setup/system-settings-logic'
import { useSystemSettings } from '../../setup/system-settings-store'
import { Panel, PanelIntro, SaveRow, SecretField } from './panel-chrome'

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
//
// This is the only caller of /api/admin/vapid/generate anywhere in the app, so
// while this panel was orphaned there was no way to configure web push at all.
export function VapidPanel() {
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
            <PanelIntro>
                The keypair browser push notifications are signed with. Generating replaces any
                existing keys and invalidates current push subscriptions.
            </PanelIntro>
            <View className="flex-row items-center gap-2">
                <Text
                    style={{ fontSize: 13 }}
                    className={configured ? 'text-success' : 'text-muted-foreground'}
                >
                    {configured ? 'Configured ✓' : 'Not configured'}
                </Text>
            </View>
            <GenerateError message={generateError} />
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

function GenerateError({ message }: { message: string | null }) {
    if (!message) return null
    return (
        <View className="rounded-lg p-2 bg-danger-soft">
            <Text className="text-xs text-danger">{message}</Text>
        </View>
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
