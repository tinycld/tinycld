import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { TermsDialog } from '@tinycld/core/components/package-terms/TermsDialog'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import {
    type PackageTermsEntry,
    usePackageTermsList,
    useTermsDecision,
} from '@tinycld/core/lib/packages/use-package-terms'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ArrowLeft } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Owner-only: the owner alone accepts terms (the create rule of
// pkg_terms_decisions). The owner lands here on launch while a package waits
// for a decision (app/a/(app)/index.tsx), and reads or accepts terms here later.
export default function PackageTermsSettings() {
    const { isOwner, isReady: roleReady } = useCurrentRole()
    const { entries, isReady } = usePackageTermsList()
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const fgColor = useThemeColor('foreground')
    // Which package's terms are open. Local: no other component needs it.
    const [openSlug, setOpenSlug] = useState<string | null>(null)
    const { decide, isPending } = useTermsDecision({ onDecided: () => setOpenSlug(null) })
    const openEntry = entries.find(e => e.slug === openSlug) ?? null

    // `role` reads null while it loads; see settings/packages.tsx.
    if (!roleReady) return null

    if (!isOwner) {
        return (
            <View className="flex-1 items-center justify-center bg-background p-5">
                <DocumentTitle pkg="Settings" title="Package terms" />
                <Text className="text-base text-muted-foreground">
                    Only the owner can accept the terms of a package.
                </Text>
            </View>
        )
    }

    const rows = entries.map(entry => (
        <TermsRow key={entry.slug} entry={entry} onOpen={() => setOpenSlug(entry.slug)} />
    ))

    return (
        <ScrollView
            className="flex-1 bg-background"
            contentContainerStyle={{ flexGrow: 1 }}
            testID="settings-package-terms"
        >
            <DocumentTitle pkg="Settings" title="Package terms" />
            <View className="w-full max-w-[720px] gap-4 p-5">
                <Pressable onPress={navigateBack} className="self-start">
                    <ArrowLeft size={24} color={fgColor} />
                </Pressable>
                <Text className="text-[22px] font-bold text-foreground">Package terms</Text>
                <Text className="text-sm text-muted-foreground">
                    A package with terms and conditions can be used only after you agree to them.
                    Until then nobody in this workspace can open it.
                </Text>
                <EmptyNote isVisible={isReady && entries.length === 0} />
                <View className="gap-3">{rows}</View>
            </View>
            <TermsDialog
                entry={openEntry}
                onClose={() => setOpenSlug(null)}
                onDecide={(entry, decision) => decide({ entry, decision })}
                isSubmitting={isPending}
            />
        </ScrollView>
    )
}

function EmptyNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="pkg-terms-empty" className="text-sm text-muted-foreground">
            No installed package has terms and conditions.
        </Text>
    )
}

const STATE_LABEL = {
    accepted: 'Accepted',
    declined: 'Declined',
    undecided: 'Not accepted yet',
} as const

function stateKeyOf(entry: PackageTermsEntry): keyof typeof STATE_LABEL {
    if (entry.state === 'accepted') return 'accepted'
    return entry.isDeclined ? 'declined' : 'undecided'
}

function TermsRow({ entry, onOpen }: { entry: PackageTermsEntry; onOpen: () => void }) {
    const stateKey = stateKeyOf(entry)
    return (
        <View
            testID={`pkg-terms-row-${entry.slug}`}
            className="flex-row flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-surface-secondary p-4"
        >
            <View className="min-w-[160px] flex-1 gap-0.5">
                <Text className="text-base font-semibold text-foreground">{entry.name}</Text>
                <Text className="text-xs text-muted-foreground">Version {entry.version}</Text>
                <Text
                    testID={`pkg-terms-state-${entry.slug}`}
                    className="text-sm text-muted-foreground"
                >
                    {STATE_LABEL[stateKey]}
                </Text>
            </View>
            <Button
                testID={`pkg-terms-open-${entry.slug}`}
                variant={entry.state === 'accepted' ? 'outline' : 'default'}
                onPress={onOpen}
            >
                <ButtonText>{entry.state === 'accepted' ? 'Read' : 'Review'}</ButtonText>
            </Button>
        </View>
    )
}
