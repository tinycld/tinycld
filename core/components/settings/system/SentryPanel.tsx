import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { sentryDsnSchema } from '../../setup/system-settings-logic'
import { useSystemSettings } from '../../setup/system-settings-store'
import { Panel, PanelIntro, SaveRow } from './panel-chrome'

const sentrySchema = z.object({ dsn: sentryDsnSchema })

export function SentryPanel() {
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
            <PanelIntro>
                The DSN errors are reported to. Leave blank to disable. Web clients pick up a change
                on their next load; native apps on their next build.
            </PanelIntro>
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
