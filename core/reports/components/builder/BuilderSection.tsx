import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronDown, ChevronRight } from 'lucide-react-native'
import { type ReactNode, useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import {
    describeProblem,
    problemsFor,
    type BuilderSection as Section,
} from '../../lib/builder/errors'
import type { Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'

function SectionProblems({ section, problems }: { section: Section; problems: Problem[] }) {
    const mine = problemsFor(section, problems)
    if (mine.length === 0) return null
    const lines = mine.map(problem => (
        <Text key={problem.path} className="text-xs text-danger">
            {describeProblem(problem)}
        </Text>
    ))
    return (
        <View
            testID={`report-builder-problems-${section}`}
            accessibilityRole="alert"
            className="gap-1 pb-2"
        >
            {lines}
        </View>
    )
}

function SectionBody({ isVisible, children }: { isVisible: boolean; children: ReactNode }) {
    if (!isVisible) return null
    return <View className="gap-3 pb-3">{children}</View>
}

interface BuilderSectionProps {
    title: string
    section: Section
    problems: Problem[]
    isOpenAtStart?: boolean
    children: ReactNode
}

// A folding part of the builder; its server problems stay in view while it is folded.
// The fold state is the section's own: nothing else reads it.
export function BuilderSection({
    title,
    section,
    problems,
    isOpenAtStart = false,
    children,
}: BuilderSectionProps) {
    const [isOpen, setIsOpen] = useState(isOpenAtStart)
    const color = useThemeColor('muted-foreground')
    const Caret = isOpen ? ChevronDown : ChevronRight
    return (
        <View className="border-b border-border">
            <Pressable
                testID={`report-builder-section-${section}`}
                accessibilityRole="button"
                aria-expanded={isOpen}
                onPress={() => setIsOpen(open => !open)}
                className="flex-row items-center gap-2"
                style={{ minHeight: REPORT_TAP_TARGET }}
            >
                <Caret size={16} color={color} />
                <Text className="text-sm font-semibold text-foreground">{title}</Text>
            </Pressable>
            <SectionProblems section={section} problems={problems} />
            <SectionBody isVisible={isOpen}>{children}</SectionBody>
        </View>
    )
}
