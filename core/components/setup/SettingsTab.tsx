import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { packageSystemSettings } from '@tinycld/core/lib/packages/derive-components'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'
import { Suspense } from 'react'
import { ActivityIndicator, Text, View } from 'react-native'
import { PageHeader, SectionLabel } from './console-ui'

export function SettingsTab({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="gap-6">
            <PageHeader
                title="Settings"
                subtitle="System-wide configuration for this deployment. These apply to the entire system, not a single organization."
            />
            <SentrySettings />
            <PackageSettings />
        </View>
    )
}

const SENTRY_DSN_KEY = 'sentry.dsn'

const sentrySchema = z.object({
    // Empty clears the DSN (disables error reporting). Otherwise require a URL so
    // a typo'd value surfaces before it silently drops events.
    dsn: z.union([z.string().url('Enter a valid Sentry DSN URL'), z.literal('')]),
})

function SentrySettings() {
    const [systemSettings] = useStore('system_settings')

    const { data: rows = [] } = useLiveQuery(query =>
        query.from({ s: systemSettings }).where(({ s }) => eq(s.key, SENTRY_DSN_KEY))
    )
    const existing = rows[0]

    const {
        control,
        handleSubmit,
        setError,
        formState: { errors, isSubmitting, isSubmitted, isDirty },
    } = useForm({
        resolver: zodResolver(sentrySchema),
        // `values` (not defaultValues) so the field reactively re-syncs when the
        // stored DSN loads async or changes server-side — RHF resets to it while
        // leaving a user's in-progress edit dirty. Avoids a useEffect+reset sync.
        values: { dsn: existing?.value ?? '' },
        mode: 'onChange',
    })

    const save = useMutation({
        mutationFn: mutation(function* (data: z.infer<typeof sentrySchema>) {
            if (existing) {
                yield systemSettings.update(existing.id, draft => {
                    draft.value = data.dsn
                })
            } else {
                yield systemSettings.insert({
                    id: newRecordId(),
                    key: SENTRY_DSN_KEY,
                    value: data.dsn,
                    is_secret: false,
                } as never)
            }
        }),
        onError: err =>
            setError('dsn', {
                message: err instanceof Error ? err.message : 'Failed to save',
            }),
    })

    const onSubmit = handleSubmit(data => save.mutate(data))

    return (
        <View className="gap-4 p-5 rounded-2xl bg-surface-secondary border border-border">
            <SectionLabel>Error reporting (Sentry)</SectionLabel>
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
            <View className="flex-row justify-end">
                <Button
                    testID="sentry-dsn-save"
                    onPress={onSubmit}
                    size="sm"
                    isDisabled={isSubmitting || !isDirty}
                >
                    <ButtonText>{save.isPending ? 'Saving…' : 'Save'}</ButtonText>
                </Button>
            </View>
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
                    const Panel = panel.Component
                    return (
                        <View
                            key={`${group.pkgSlug}:${panel.slug}`}
                            className="gap-4 p-5 rounded-2xl bg-surface-secondary border border-border"
                        >
                            <SectionLabel>
                                {group.packageName} — {panel.label}
                            </SectionLabel>
                            <Suspense
                                fallback={
                                    <View className="py-6 items-center">
                                        <ActivityIndicator />
                                    </View>
                                }
                            >
                                <Panel />
                            </Suspense>
                        </View>
                    )
                })
            )}
        </>
    )
}
