import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Check, CircleAlert, Loader2 } from 'lucide-react-native'
import { useEffect, useRef } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { type OperationStatus, type ProgressStep, useInstallProgress } from './use-install-progress'

interface InstallProgressModalProps {
    isVisible: boolean
    jobId: string | null
    authToken: string
    onClose: () => void
    onComplete: () => void
}

export function InstallProgressModal({
    isVisible,
    jobId,
    authToken,
    onClose,
    onComplete,
}: InstallProgressModalProps) {
    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const successColor = useThemeColor('success')
    const dangerColor = useThemeColor('danger')

    const { steps, status, error } = useInstallProgress(isVisible, jobId, authToken, onComplete)
    const scrollRef = useRef<ScrollView>(null)

    // Auto-scroll the step log to the bottom as new steps stream in. The effect
    // reads stepCount so it genuinely depends on it: each appended step bumps the
    // count, re-runs the effect, and scrolls to the newest line.
    const stepCount = steps.length
    useEffect(() => {
        if (stepCount > 0) scrollRef.current?.scrollToEnd({ animated: true })
    }, [stepCount])

    if (!isVisible) return null

    const progress = steps[steps.length - 1]?.progress ?? 0

    return (
        <View className="rounded-xl border border-border bg-surface-secondary overflow-hidden">
            <View className="p-4 gap-4">
                <View className="flex-row justify-between items-center">
                    <Text className="text-base font-semibold text-foreground">
                        {titleForStatus(status)}
                    </Text>
                    <StatusIcon
                        status={status}
                        successColor={successColor}
                        dangerColor={dangerColor}
                        mutedColor={mutedColor}
                    />
                </View>

                <ProgressBar
                    progress={progress}
                    status={status}
                    successColor={successColor}
                    dangerColor={dangerColor}
                />

                <ScrollView
                    ref={scrollRef}
                    className="rounded-lg border border-border bg-surface-secondary"
                    style={{ maxHeight: 300 }}
                >
                    <View className="p-3 gap-1">
                        {steps.map((step, i) => (
                            <StepLine
                                key={`${step.progress}-${step.step}`}
                                step={step}
                                isLatest={i === steps.length - 1}
                                status={status}
                                fgColor={fgColor}
                                mutedColor={mutedColor}
                                successColor={successColor}
                                dangerColor={dangerColor}
                            />
                        ))}
                    </View>
                </ScrollView>

                <ErrorDisplay error={error} />

                <CloseButton isVisible={status !== 'running'} onPress={onClose} />
            </View>
        </View>
    )
}

function titleForStatus(status: OperationStatus): string {
    if (status === 'success') return 'Installation Complete'
    if (status === 'failed') return 'Installation Failed'
    return 'Installing Package...'
}

function StatusIcon({
    status,
    successColor,
    dangerColor,
    mutedColor,
}: {
    status: string
    successColor: string
    dangerColor: string
    mutedColor: string
}) {
    if (status === 'success') return <Check size={20} color={successColor} />
    if (status === 'failed') return <CircleAlert size={20} color={dangerColor} />
    return <Loader2 size={20} color={mutedColor} />
}

function ProgressBar({
    progress,
    status,
    successColor,
    dangerColor,
}: {
    progress: number
    status: string
    successColor: string
    dangerColor: string
}) {
    const barColor = status === 'failed' ? dangerColor : successColor

    return (
        <View className="h-2 rounded-full bg-border overflow-hidden">
            <View
                testID="install-progress-fill"
                // The numeric progress is exposed as ARIA value attributes so e2e can
                // read it directly off `aria-valuenow` (the visual width is an inline %
                // style that's awkward to assert on). Proves the SSE stream is advancing.
                // NOTE: react-native-web 0.21 dropped support for the object form
                // `accessibilityValue={{ now, min, max }}` — it only forwards the
                // flattened `aria-value*` props (with a `progressbar` role), so the
                // object form silently emits no `aria-valuenow` and the e2e read sees
                // `null`. Pass the flattened props directly.
                role="progressbar"
                aria-valuenow={progress}
                aria-valuemin={0}
                aria-valuemax={100}
                className="h-full rounded-full"
                style={{
                    width: `${progress}%`,
                    backgroundColor: barColor,
                }}
            />
        </View>
    )
}

function StepLine({
    step,
    isLatest,
    status,
    fgColor,
    mutedColor,
    successColor,
    dangerColor,
}: {
    step: ProgressStep
    isLatest: boolean
    status: string
    fgColor: string
    mutedColor: string
    successColor: string
    dangerColor: string
}) {
    const isActive = isLatest && status === 'running'
    const isFailed = step.message.startsWith('FAILED')
    const color = isActive ? fgColor : isFailed ? dangerColor : mutedColor

    return (
        <View className="flex-row gap-2 items-start">
            <Text
                className="text-[11px] text-muted-foreground"
                style={{ fontVariant: ['tabular-nums'], minWidth: 32 }}
            >
                {step.progress}%
            </Text>
            <View className="mt-1.5">
                {isActive ? (
                    <Loader2 size={10} color={fgColor} />
                ) : (
                    <Check size={10} color={successColor} />
                )}
            </View>
            <Text className="text-xs flex-1" style={{ color }}>
                {step.message}
            </Text>
        </View>
    )
}

function ErrorDisplay({ error }: { error: string | null }) {
    if (!error) return null
    return (
        <View className="rounded-lg p-3 bg-danger-soft">
            <Text className="text-[13px] text-danger">{error}</Text>
        </View>
    )
}

function CloseButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Pressable onPress={onPress} className="self-end px-3 py-2 rounded-lg bg-border">
            <Text className="text-[13px] font-semibold text-muted-foreground">Close</Text>
        </Pressable>
    )
}
