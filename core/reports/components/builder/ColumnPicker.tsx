import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Popover } from '@tinycld/core/ui/popover'
import { ArrowLeft, ChevronRight } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { type Catalog, MAX_HOPS, sourceAtLinks } from '../../lib/builder/columns'
import type { SpecHop } from '../../lib/spec'
import type { CatalogLink } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'

const PICKER_WIDTH = 340
const LIST_MAX_HEIGHT = 380

interface PickRowProps {
    label: string
    hint?: string
    testID: string
    onPress: () => void
}

function PickRow({ label, hint, testID, onPress }: PickRowProps) {
    const muted = useThemeColor('muted-foreground')
    return (
        <Pressable
            testID={testID}
            accessibilityRole="button"
            onPress={onPress}
            className="flex-row items-center gap-2 rounded-md px-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text numberOfLines={1} className="min-w-0 flex-1 text-sm text-foreground">
                {label}
            </Text>
            <Text className="text-xs text-muted-foreground">{hint ?? ''}</Text>
            <ChevronRight size={14} color={muted} />
        </Pressable>
    )
}

function BackRow({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    const muted = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable
            testID="report-builder-pick-back"
            accessibilityRole="button"
            onPress={onPress}
            className="flex-row items-center gap-1 px-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <ArrowLeft size={14} color={muted} />
            <Text className="text-sm text-muted-foreground">Back</Text>
        </Pressable>
    )
}

interface ManyChoiceProps {
    link: CatalogLink | null
    onTotal: () => void
    onNest: () => void
}

// A to-many link reaches many rows: they are totalled onto each row, or shown as child rows.
function ManyChoice({ link, onTotal, onNest }: ManyChoiceProps) {
    if (!link) return null
    return (
        <View className="gap-2 p-2">
            <Text className="text-sm text-foreground">{`Each row has many ${link.label}.`}</Text>
            <Button testID="report-builder-pick-total" variant="outline" onPress={onTotal}>
                <ButtonText>Add as total</ButtonText>
            </Button>
            <Button testID="report-builder-pick-nest" variant="outline" onPress={onNest}>
                <ButtonText>Show as child rows</ButtonText>
            </Button>
        </View>
    )
}

interface OpenPickerProps {
    catalog: Catalog
    baseKey: string
    toOneOnly: boolean
    onPick(path: SpecHop[]): void
}

function linkHint(link: CatalogLink): string {
    return link.cardinality === 'toMany' ? 'many' : 'one'
}

// The links walked so far and a to-many link waiting for its choice are this open
// picker's own state; a new open starts at the source.
function OpenPicker({ catalog, baseKey, toOneOnly, onPick }: OpenPickerProps) {
    const [hops, setHops] = useState<SpecHop[]>([])
    const [pending, setPending] = useState<CatalogLink | null>(null)
    const source = sourceAtLinks(catalog, baseKey, hops)
    const canFollow = hops.length < MAX_HOPS
    const back = () => {
        if (pending) setPending(null)
        else setHops(hops.slice(0, -1))
    }
    const follow = (link: CatalogLink) => {
        if (link.cardinality === 'toMany') setPending(link)
        else setHops([...hops, { link: link.key }])
    }
    const choose = (mode: 'aggregate' | 'nest') => {
        if (!pending) return
        setHops([...hops, { link: pending.key, mode }])
        setPending(null)
    }
    const shownFields = pending ? [] : (source?.fields ?? [])
    const fields = shownFields.map(field => (
        <PickRow
            key={field.key}
            label={field.label}
            testID={`report-builder-pick-field-${field.key}`}
            onPress={() => onPick([...hops, { field: field.key }])}
        />
    ))
    const followable = pending || !canFollow ? [] : (source?.links ?? [])
    const offered = followable.filter(link => !toOneOnly || link.cardinality === 'toOne')
    const links = offered.map(link => (
        <PickRow
            key={link.key}
            label={link.label}
            hint={linkHint(link)}
            testID={`report-builder-pick-link-${link.key}`}
            onPress={() => follow(link)}
        />
    ))
    return (
        <View className="gap-1">
            <BackRow isVisible={hops.length > 0 || pending !== null} onPress={back} />
            <Text className="px-2 text-xs font-semibold uppercase text-muted-foreground">
                {source?.label ?? ''}
            </Text>
            <ManyChoice
                link={pending}
                onTotal={() => choose('aggregate')}
                onNest={() => choose('nest')}
            />
            <ScrollView style={{ maxHeight: LIST_MAX_HEIGHT }} nestedScrollEnabled>
                {fields}
                {links}
            </ScrollView>
        </View>
    )
}

function PickerBody({ isVisible, ...props }: OpenPickerProps & { isVisible: boolean }) {
    if (!isVisible) return null
    return <OpenPicker {...props} />
}

export interface ColumnPickerProps {
    catalog: Catalog
    baseKey: string
    onPick(path: SpecHop[]): void
    label: string
    testID: string
    /** For a filter field or a pivot: one value per row, so no to-many link. */
    toOneOnly?: boolean
}

// A tree of the source's fields and links: a to-one link opens in place, a to-many
// link offers "Add as total" or "Show as child rows". The open flag is the picker's
// own UI state.
export function ColumnPicker({
    catalog,
    baseKey,
    onPick,
    label,
    testID,
    toOneOnly = false,
}: ColumnPickerProps) {
    const [isOpen, setIsOpen] = useState(false)
    const trigger = (
        <Button testID={testID} variant="outline" size="sm">
            <ButtonText>{label}</ButtonText>
        </Button>
    )
    const pick = (path: SpecHop[]) => {
        setIsOpen(false)
        onPick(path)
    }
    return (
        <Popover
            trigger={trigger}
            isOpen={isOpen}
            onOpenChange={setIsOpen}
            title={label}
            sheetSide="bottom"
            width={PICKER_WIDTH}
            testID="report-builder-picker"
        >
            <PickerBody
                isVisible={isOpen}
                catalog={catalog}
                baseKey={baseKey}
                toOneOnly={toOneOnly}
                onPick={pick}
            />
        </Popover>
    )
}
