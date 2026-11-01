import { LoadingState } from '@tinycld/core/components/LoadingState'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Text, View } from 'react-native'
import { describeProblem } from '../lib/builder/errors'
import type { EmptyAction } from '../lib/params'
import type { Problem } from '../lib/types'
import { REPORT_TAP_TARGET } from './ReportHeader'

export function LoadingBody({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-loading" className="flex-1">
            <LoadingState />
        </View>
    )
}

type StateAction = { label: string; testID: string; onPress: () => void }

interface MessageBodyProps {
    isVisible: boolean
    /** The state's test id; its message is `<testID>-message`. */
    testID: string
    message: string
    action?: StateAction | null
}

function ActionButton({ action }: { action: StateAction | null | undefined }) {
    if (!action) return null
    return (
        <Button
            testID={action.testID}
            variant="outline"
            style={{ minHeight: REPORT_TAP_TARGET }}
            onPress={action.onPress}
        >
            <ButtonText>{action.label}</ButtonText>
        </Button>
    )
}

/** A state with a message and at most one action, drawn in place of the table. */
export function MessageBody({ isVisible, testID, message, action }: MessageBodyProps) {
    if (!isVisible) return null
    return (
        <View testID={testID} accessibilityRole="alert" className="items-center gap-3 p-6">
            <Text
                testID={`${testID}-message`}
                className="text-center text-base font-medium text-foreground"
            >
                {message}
            </Text>
            <ActionButton action={action} />
        </View>
    )
}

interface ProblemsBodyProps {
    isVisible: boolean
    testID: string
    message: string
    problems: Problem[]
    action: StateAction | null
}

// A report that cannot run names each missing or invalid part.
export function ProblemsBody({ isVisible, testID, message, problems, action }: ProblemsBodyProps) {
    if (!isVisible) return null
    const lines = problems.map((problem, i) => (
        <Text
            key={problem.path}
            testID={`${testID}-problem-${i}`}
            className="text-sm text-muted-foreground"
        >
            {describeProblem(problem)}
        </Text>
    ))
    return (
        <View testID={testID} accessibilityRole="alert" className="items-center gap-2 p-6">
            <Text
                testID={`${testID}-message`}
                className="text-center text-base font-medium text-foreground"
            >
                {message}
            </Text>
            {lines}
            <ActionButton action={action} />
        </View>
    )
}

interface EmptyBodyProps {
    isVisible: boolean
    message: string
    action: EmptyAction | null
    onAction: () => void
}

export function EmptyBody({ isVisible, message, action, onAction }: EmptyBodyProps) {
    const button = action
        ? { label: action.label, testID: 'report-empty-action', onPress: onAction }
        : null
    return (
        <MessageBody
            isVisible={isVisible}
            testID="report-empty"
            message={message}
            action={button}
        />
    )
}
