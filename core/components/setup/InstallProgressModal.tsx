import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Check, CircleAlert } from 'lucide-react-native'
import { useEffect, useRef, useState } from 'react'
import { ActivityIndicator, Platform, Pressable, ScrollView, Text, View } from 'react-native'
import {
    collapseSteps,
    type DebugLine as DebugLineData,
    debugLines,
    type StepRow,
} from './progress-view'
import { type OperationStatus, type ProgressStep, useInstallProgress } from './use-install-progress'
import { type ServerSwitch, useServerSwitch } from './use-server-switch'

// The job the panel is tracking, which decides its title: a background job is
// the same shape whichever way it was started, and the progress stream does not
// say which, so the caller that started it tells the panel.
export type ProgressAction = 'install' | 'uninstall' | 'apply' | 'revert'

interface InstallProgressModalProps {
    isVisible: boolean
    jobId: string | null
    action: ProgressAction
    authToken: string
    onClose: () => void
    onComplete: () => void
}

export function InstallProgressModal({
    isVisible,
    jobId,
    action,
    authToken,
    onClose,
    onComplete,
}: InstallProgressModalProps) {
    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const successColor = useThemeColor('success')
    const dangerColor = useThemeColor('danger')

    const { steps, status, error } = useInstallProgress(isVisible, jobId, authToken, onComplete)
    const rows = collapseSteps(steps, status)
    const serverSwitch = useServerSwitch(isVisible, jobId, status)
    // The raw command output means nothing to most users, so it stays hidden
    // until someone asks for it (support, or an admin who knows the build).
    const [isDebugLogVisible, setDebugLogVisible] = useState(false)
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
    const colors = { fgColor, mutedColor, successColor, dangerColor }

    return (
        <View className="rounded-xl border border-border bg-surface-secondary overflow-hidden">
            <View className="p-4 gap-4">
                <View className="flex-row justify-between items-center">
                    <Text className="text-base font-semibold text-foreground">
                        {titleFor(action, status)}
                    </Text>
                    <StatusIcon
                        status={status}
                        successColor={successColor}
                        dangerColor={dangerColor}
                    />
                </View>

                <ProgressBar
                    testID="install-progress-fill"
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
                    <StepList
                        isVisible={!isDebugLogVisible}
                        rows={rows}
                        status={status}
                        {...colors}
                    />
                    <DebugLog
                        isVisible={isDebugLogVisible}
                        lines={debugLines(steps)}
                        status={status}
                        {...colors}
                    />
                </ScrollView>

                <FailureHint isVisible={status === 'failed' && !isDebugLogVisible} />
                <ErrorDisplay error={isDebugLogVisible ? error : null} />

                <Pressable
                    onPress={() => setDebugLogVisible(v => !v)}
                    className="self-start"
                    accessibilityRole="button"
                >
                    <Text className="text-[13px] text-muted-foreground underline">
                        {isDebugLogVisible ? 'Hide debug log' : 'View debug log'}
                    </Text>
                </Pressable>

                <ProgressFooter
                    isVisible={status !== 'running'}
                    needsReload={status === 'success'}
                    serverSwitch={serverSwitch}
                    onClose={onClose}
                />
            </View>
        </View>
    )
}

const TITLES: Record<ProgressAction, Record<OperationStatus, string>> = {
    install: {
        running: 'Installing Package...',
        success: 'Installation Complete',
        failed: 'Installation Failed',
    },
    uninstall: {
        running: 'Uninstalling Package...',
        success: 'Uninstall Complete',
        failed: 'Uninstall Failed',
    },
    apply: {
        running: 'Applying Version Changes...',
        success: 'Version Changes Applied',
        failed: 'Version Change Failed',
    },
    revert: {
        running: 'Reverting Build...',
        success: 'Revert Complete',
        failed: 'Revert Failed',
    },
}

// The install log records a job's kind under its own vocabulary; the panel
// titles by the four kinds it distinguishes.
export function progressActionFor(logAction: string): ProgressAction {
    if (logAction === 'uninstall' || logAction === 'revert') return logAction
    if (logAction === 'version_change') return 'apply'
    return 'install'
}

export function titleFor(action: ProgressAction, status: OperationStatus): string {
    return TITLES[action][status]
}

// Only the terminal states get a header icon. While the job runs, the current
// step already carries its own spinner, so a second one here is noise.
function StatusIcon({
    status,
    successColor,
    dangerColor,
}: {
    status: string
    successColor: string
    dangerColor: string
}) {
    if (status === 'success') return <Check size={20} color={successColor} />
    if (status === 'failed') return <CircleAlert size={20} color={dangerColor} />
    return null
}

function ProgressBar({
    testID,
    progress,
    status,
    successColor,
    dangerColor,
}: {
    testID?: string
    progress: number
    status: string
    successColor: string
    dangerColor: string
}) {
    const barColor = status === 'failed' ? dangerColor : successColor

    return (
        <View className="h-2 rounded-full bg-border overflow-hidden">
            <View
                testID={testID}
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

interface LineColors {
    fgColor: string
    mutedColor: string
    successColor: string
    dangerColor: string
}

function StepList({
    isVisible,
    rows,
    status,
    ...colors
}: LineColors & { isVisible: boolean; rows: StepRow[]; status: OperationStatus }) {
    if (!isVisible) return null
    return (
        <View className="p-3 gap-2">
            {rows.map(row => (
                <StepRowLine key={row.id} row={row} status={status} {...colors} />
            ))}
        </View>
    )
}

function StepRowLine({
    row,
    status,
    fgColor,
    mutedColor,
    successColor,
    dangerColor,
}: LineColors & { row: StepRow; status: OperationStatus }) {
    const color = row.isCurrent ? fgColor : row.isFailed ? dangerColor : mutedColor
    const label = row.isFailed ? `${row.step} failed` : row.step
    return (
        <View className="gap-1">
            <View className="flex-row gap-2 items-center">
                <RowIcon
                    row={row}
                    fgColor={fgColor}
                    successColor={successColor}
                    dangerColor={dangerColor}
                />
                <Text className="text-[13px] flex-1" style={{ color }}>
                    {label}
                </Text>
            </View>
            <StepProgress
                row={row}
                status={status}
                successColor={successColor}
                dangerColor={dangerColor}
            />
        </View>
    )
}

function RowIcon({
    row,
    fgColor,
    successColor,
    dangerColor,
}: Omit<LineColors, 'mutedColor'> & { row: StepRow }) {
    if (row.isFailed) return <CircleAlert size={12} color={dangerColor} />
    if (row.isCurrent) return <StepSpinner color={fgColor} />
    return <Check size={12} color={successColor} />
}

// ActivityIndicator, not a lucide icon: lucide renders a static glyph, so a
// Loader2 here sat frozen. Its smallest size is 20px, scaled down to sit on the
// same 12px baseline as the Check and CircleAlert it alternates with.
function StepSpinner({ color }: { color: string }) {
    return (
        <View className="w-3 h-3 items-center justify-center">
            <ActivityIndicator size="small" color={color} style={{ transform: [{ scale: 0.6 }] }} />
        </View>
    )
}

// Bundling runs for minutes inside a few points of the overall bar; its own
// bar shows the step is still moving.
function StepProgress({
    row,
    status,
    successColor,
    dangerColor,
}: Pick<LineColors, 'successColor' | 'dangerColor'> & { row: StepRow; status: OperationStatus }) {
    if (!row.isCurrent || row.stepProgress === null) return null
    return (
        <View className="pl-5">
            <ProgressBar
                progress={row.stepProgress}
                status={status}
                successColor={successColor}
                dangerColor={dangerColor}
            />
        </View>
    )
}

function DebugLog({
    isVisible,
    lines,
    status,
    ...colors
}: LineColors & { isVisible: boolean; lines: DebugLineData[]; status: OperationStatus }) {
    if (!isVisible) return null
    const latestId = lines[lines.length - 1]?.id
    return (
        <View className="p-3 gap-1">
            {lines.map(line => (
                <DebugLine
                    key={line.id}
                    step={line}
                    isLatest={line.id === latestId}
                    status={status}
                    {...colors}
                />
            ))}
        </View>
    )
}

function DebugLine({
    step,
    isLatest,
    status,
    fgColor,
    mutedColor,
    dangerColor,
}: LineColors & { step: ProgressStep; isLatest: boolean; status: OperationStatus }) {
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
            <Text className="text-xs flex-1" style={{ color, fontFamily: 'monospace' }}>
                {step.step}: {step.message}
            </Text>
        </View>
    )
}

function FailureHint({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="text-[13px] text-muted-foreground">
            Open the debug log to see what went wrong.
        </Text>
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

// A completed install/uninstall swapped the deployment onto a NEW build, so the
// running app is on the previous one: its bundle has no screens, nav entry or
// route chunks for a package that was just added. The package registry arrives
// over realtime so the list below updates, but the app itself cannot until it
// picks up the new bundle — leaving a sidebar that silently omits the package,
// which reads as a failed install.
//
// How that happens differs by platform, so the footer says which:
//
//   web     nothing fetches a new bundle on its own. Offer a reload — offered,
//           not forced, because the admin may be mid-task elsewhere;
//           useChunkLoadRecovery still catches a stale chunk if they carry on.
//   native  useAppUpdates already downloads and stages the org's new bundle and
//           reloads onto it on the next foreground, so there is nothing to
//           press. Say when it lands instead of offering a dead button.
//
// NOT reloadJsContext(): that helper restarts the NATIVE JS context for a
// server switch and throws on web ("web has no JS context to restart"), which
// is the only platform with a button here. The two are complements.
//
// Either way the hint waits for useServerSwitch: the job succeeds before the
// new server answers, and a reload or reopen before then loads the previous
// build again.
function ProgressFooter({
    isVisible,
    needsReload,
    serverSwitch,
    onClose,
}: {
    isVisible: boolean
    needsReload: boolean
    serverSwitch: ServerSwitch
    onClose: () => void
}) {
    if (!isVisible) return null
    const isWeb = Platform.OS === 'web' && typeof window !== 'undefined'
    const canReload = needsReload && isWeb && serverSwitch !== 'waiting'
    return (
        <View className="gap-2">
            <NewBuildHint isVisible={needsReload} text={newBuildHint(isWeb, serverSwitch)} />
            <View className="flex-row justify-end gap-2">
                <Pressable onPress={onClose} className="px-3 py-2 rounded-lg bg-border">
                    <Text className="text-[13px] font-semibold text-muted-foreground">Close</Text>
                </Pressable>
                <ReloadButton isVisible={canReload} />
            </View>
        </View>
    )
}

const WAITING_HINT = 'Waiting for the server to start the new build…'

const NEW_BUILD_HINTS: Record<'web' | 'native', Record<ServerSwitch, string>> = {
    web: {
        waiting: WAITING_HINT,
        ready: 'Reload to finish — this tab is still running the previous build.',
        unconfirmed:
            'The server is still switching to the new build. Reload to finish; if the previous build loads, wait a minute and reload again.',
    },
    native: {
        waiting: WAITING_HINT,
        ready: 'The new build is applied the next time you reopen the app.',
        unconfirmed:
            'The server is still switching to the new build. It is applied the next time you reopen the app after the switch.',
    },
}

function newBuildHint(isWeb: boolean, serverSwitch: ServerSwitch): string {
    return NEW_BUILD_HINTS[isWeb ? 'web' : 'native'][serverSwitch]
}

function NewBuildHint({ isVisible, text }: { isVisible: boolean; text: string }) {
    if (!isVisible) return null
    return <Text className="text-[13px] text-muted-foreground">{text}</Text>
}

function ReloadButton({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Pressable
            onPress={() => window.location.reload()}
            className="px-3 py-2 rounded-lg bg-primary"
        >
            <Text className="text-[13px] font-semibold text-primary-foreground">Reload</Text>
        </Pressable>
    )
}
