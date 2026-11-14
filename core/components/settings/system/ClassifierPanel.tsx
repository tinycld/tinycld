import { useClassifierUsage } from '@tinycld/core/lib/classifier/use-classifier-usage'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, useForm, zodResolver } from '@tinycld/core/ui/form'
import { Text, View } from 'react-native'
import { useSystemSettings } from '../../setup/system-settings-store'
import {
    CLASSIFIER_DEFAULTS,
    type ClassifierFormValues,
    classifierSchema,
} from './classifier-schema'
import { Panel, PanelIntro, SaveRow } from './panel-chrome'

type VerifyResponse = { ok: boolean; error: string }

const STATUS_MESSAGES: Record<string, string> = {
    auth_failed:
        'OpenRouter rejected the key. Classification is paused until you verify a working key.',
    no_credits:
        'OpenRouter reports no credits. Classification is paused until you add credits and verify the connection.',
    access_denied:
        'OpenRouter denied access to the model. Classification is paused until you verify the connection.',
}

function useClassifierVerify() {
    const mutation = useMutation({
        mutationFn: async (): Promise<VerifyResponse> =>
            await pb.send('/api/classifier/verify', { method: 'POST' }),
    })
    return {
        run: () => mutation.mutate(),
        isPending: mutation.isPending,
        result: mutation.data,
    }
}

function useClassifierSave(
    form: ReturnType<typeof useForm<ClassifierFormValues>>,
    upsert: ReturnType<typeof useSystemSettings>['upsert']
) {
    return form.handleSubmit(async data => {
        try {
            await upsert.mutateAsync({
                key: 'classifier.monthly_budget_usd',
                value: String(Number(data.budget)),
                isSecret: false,
            })
            await upsert.mutateAsync({
                key: 'classifier.full_evidence_threshold',
                value: String(Number(data.fullThreshold)),
                isSecret: false,
            })
            await upsert.mutateAsync({
                key: 'classifier.auto_action_threshold',
                value: String(Number(data.autoActionThreshold)),
                isSecret: false,
            })
            if (data.apiKey) {
                await upsert.mutateAsync({
                    key: 'classifier.api_key',
                    value: data.apiKey,
                    isSecret: true,
                })
            }
            form.reset({ ...data, apiKey: '' })
        } catch (err) {
            form.setError('root', {
                type: 'server',
                message: err instanceof Error ? err.message : 'Failed to save',
            })
        }
    })
}

function ClassifierStatusBanner({ status }: { status: string }) {
    const message = STATUS_MESSAGES[status]
    if (!message) return null
    return (
        <View testID="classifier-status" className="p-3 rounded-lg bg-destructive/10">
            <Text className="text-destructive" style={{ fontSize: 13 }}>
                {message}
            </Text>
        </View>
    )
}

function MonthSpend({
    costUsd,
    calls,
    budget,
}: {
    costUsd: number
    calls: number
    budget: number
}) {
    const budgetText = Number.isFinite(budget) ? budget.toFixed(2) : '0.00'
    return (
        <Text
            testID="classifier-month-spend"
            className="text-muted-foreground"
            style={{ fontSize: 13 }}
        >
            {`This month: $${costUsd.toFixed(4)} of $${budgetText} (${calls} calls)`}
        </Text>
    )
}

function VerifyResult({ result }: { result: VerifyResponse | undefined }) {
    if (!result) return null
    return (
        <Text
            testID="classifier-verify-result"
            className={result.ok ? 'text-foreground' : 'text-destructive'}
            style={{ fontSize: 13 }}
        >
            {result.ok ? 'Connected' : result.error}
        </Text>
    )
}

function UnsavedKeyHint({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text
            testID="classifier-unsaved-key-hint"
            className="text-muted-foreground"
            style={{ fontSize: 13 }}
        >
            Save the key before you verify it. Verify tests the saved key.
        </Text>
    )
}

function RemoveKeyButton({
    isVisible,
    isPending,
    onPress,
}: {
    isVisible: boolean
    isPending: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <ServerActionButton
            testID="classifier-remove-key"
            onPress={onPress}
            isDisabled={isPending}
            variant="outline"
            size="sm"
        >
            <ButtonText>Remove key</ButtonText>
        </ServerActionButton>
    )
}

export function ClassifierPanel() {
    const { byKey, upsert, remove } = useSystemSettings('classifier')
    const usage = useClassifierUsage()
    const verify = useClassifierVerify()
    const status = byKey.get('classifier.status')?.value || 'ok'
    const storedBudget = Number(
        byKey.get('classifier.monthly_budget_usd')?.value || CLASSIFIER_DEFAULTS.budget
    )
    const form = useForm({
        resolver: zodResolver(classifierSchema),
        values: {
            apiKey: '',
            budget: String(storedBudget),
            fullThreshold: String(
                byKey.get('classifier.full_evidence_threshold')?.value ||
                    CLASSIFIER_DEFAULTS.fullThreshold
            ),
            autoActionThreshold: String(
                byKey.get('classifier.auto_action_threshold')?.value ||
                    CLASSIFIER_DEFAULTS.autoActionThreshold
            ),
        },
        mode: 'onChange',
    })
    const onSubmit = useClassifierSave(form, upsert)
    const hasKey = byKey.has('classifier.api_key')
    const hasUnsavedKey = form.watch('apiKey') !== ''
    return (
        <Panel label="AI classification">
            <PanelIntro>
                Classify incoming items with the TypeSafe Jev model through OpenRouter. Leave the
                key blank to keep the feature off. Each classified item sends its metadata, and
                sometimes its text, to OpenRouter.
            </PanelIntro>
            <ClassifierStatusBanner status={status} />
            <FormErrorSummary
                errors={form.formState.errors}
                isEnabled={form.formState.isSubmitted}
            />
            <TextInput
                control={form.control}
                name="apiKey"
                label="OpenRouter API key"
                secureTextEntry
                autoCapitalize="none"
                placeholder={hasKey ? 'Saved — enter a new key to replace it' : 'sk-or-…'}
                testID="classifier-api-key"
            />
            <TextInput
                control={form.control}
                name="budget"
                label="Monthly budget (USD)"
                keyboardType="decimal-pad"
                testID="classifier-budget"
            />
            <TextInput
                control={form.control}
                name="fullThreshold"
                label="Read the full text below this confidence"
                keyboardType="decimal-pad"
                testID="classifier-full-threshold"
            />
            <TextInput
                control={form.control}
                name="autoActionThreshold"
                label="Automatic action threshold"
                keyboardType="decimal-pad"
                testID="classifier-auto-action-threshold"
            />
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                Apps take their automatic action (such as archiving) only at or above this
                confidence.
            </Text>
            <MonthSpend costUsd={usage.costUsd} calls={usage.calls} budget={storedBudget} />
            <SaveRow
                testID="classifier-save"
                onPress={onSubmit}
                isPending={upsert.isPending}
                isDisabled={!form.formState.isDirty}
            />
            <ServerActionButton
                testID="classifier-verify"
                onPress={verify.run}
                isDisabled={!hasKey || hasUnsavedKey || verify.isPending}
                size="sm"
            >
                <ButtonText>{verify.isPending ? 'Verifying…' : 'Verify connection'}</ButtonText>
            </ServerActionButton>
            <UnsavedKeyHint isVisible={hasUnsavedKey} />
            <RemoveKeyButton
                isVisible={hasKey}
                isPending={remove.isPending}
                onPress={() => remove.mutate('classifier.api_key')}
            />
            <VerifyResult result={verify.result} />
        </Panel>
    )
}
