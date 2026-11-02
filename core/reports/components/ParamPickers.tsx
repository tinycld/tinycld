import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { NumberInput, TextInput, useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { Popover } from '@tinycld/core/ui/popover'
import { Check } from 'lucide-react-native'
import type { ReactElement } from 'react'
import {
    ActivityIndicator,
    type NativeScrollEvent,
    type NativeSyntheticEvent,
    Pressable,
    ScrollView,
    Text,
    View,
} from 'react-native'
import { type OptionsSource, useParamOptions } from '../hooks/useParamOptions'
import { textFromCents } from '../lib/money'
import {
    type AmountValue,
    amountParamValue,
    amountValueSchema,
    choicesSchema,
    type NumberValue,
    numberValueSchema,
    type SourceChoiceValues,
    sourceChoiceSchema,
    textValueSchema,
} from '../lib/param-forms'
import { optionSections, optionText, toggleGroup } from '../lib/param-options'
import type { ParamValue } from '../lib/params'
import type { ParamDef } from '../lib/spec'
import type { ParamOption, ParamOptionGroup } from '../lib/types'
import { REPORT_TAP_TARGET } from './ReportHeader'
import { isNearEnd } from './use-row-window'

const PICKER_WIDTH = 340
const LIST_STYLE = { maxHeight: 360 }
const TARGET_STYLE = { minHeight: REPORT_TAP_TARGET }

function ApplyButton({ isDisabled, onPress }: { isDisabled: boolean; onPress: () => void }) {
    return (
        <View className="border-t border-border pt-2">
            <Button
                testID="report-filter-apply"
                style={TARGET_STYLE}
                isDisabled={isDisabled}
                onPress={onPress}
            >
                <ButtonText>Apply</ButtonText>
            </Button>
        </View>
    )
}

function CheckMark({ isVisible }: { isVisible: boolean }) {
    const color = useThemeColor('primary-foreground')
    if (!isVisible) return null
    return <Check size={14} color={color} />
}

interface OptionRowProps {
    value: string
    label: string
    isChecked: boolean
    isRadio: boolean
    onPress: () => void
}

function OptionRow({ value, label, isChecked, isRadio, onPress }: OptionRowProps) {
    const tone = isChecked ? 'border-primary bg-primary' : 'border-border bg-transparent'
    const shape = isRadio ? 'rounded-full' : 'rounded'
    const role = isRadio ? 'radio' : 'checkbox'
    return (
        <Pressable
            testID={`report-filter-option-${value}`}
            accessibilityRole={role}
            accessibilityState={{ checked: isChecked }}
            aria-checked={isChecked}
            onPress={onPress}
            className="flex-row items-center gap-3 rounded-md px-2"
            style={TARGET_STYLE}
        >
            <View className={`h-5 w-5 items-center justify-center border ${shape} ${tone}`}>
                <CheckMark isVisible={isChecked} />
            </View>
            <Text numberOfLines={1} className="min-w-0 flex-1 text-sm text-foreground">
                {label}
            </Text>
        </Pressable>
    )
}

interface BodyProps {
    def: ParamDef
    value: ParamValue | null
    onApply(value: ParamValue): void
}

// One choice applies when it is pressed.
function SingleChoiceBody({ def, value, onApply }: BodyProps) {
    const current = value?.kind === 'choice' ? value.values[0] : undefined
    const rows = (def.options ?? []).map(option => (
        <OptionRow
            key={option.value}
            value={option.value}
            label={option.label}
            isChecked={option.value === current}
            isRadio
            onPress={() => onApply({ kind: 'choice', values: [option.value] })}
        />
    ))
    return (
        <ScrollView style={LIST_STYLE} nestedScrollEnabled>
            {rows}
        </ScrollView>
    )
}

// Several choices apply on Apply; an empty selection is not a filter.
function MultiChoiceBody({ def, value, onApply }: BodyProps) {
    const form = useForm({
        resolver: zodResolver(choicesSchema),
        mode: 'onChange',
        defaultValues: { values: value?.kind === 'choice' ? value.values : [] },
    })
    const values = useWatch({ control: form.control, name: 'values' })
    const toggle = (option: string) => {
        const next = values.includes(option)
            ? values.filter(v => v !== option)
            : [...values, option]
        form.setValue('values', next, { shouldValidate: true })
    }
    const rows = (def.options ?? []).map(option => (
        <OptionRow
            key={option.value}
            value={option.value}
            label={option.label}
            isChecked={values.includes(option.value)}
            isRadio={false}
            onPress={() => toggle(option.value)}
        />
    ))
    const apply = () =>
        void form.handleSubmit(submitted => onApply({ kind: 'choice', values: submitted.values }))()
    return (
        <View className="gap-2">
            <ScrollView style={LIST_STYLE} nestedScrollEnabled>
                {rows}
            </ScrollView>
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function TextBody({ def, value, onApply }: BodyProps) {
    const form = useForm({
        resolver: zodResolver(textValueSchema),
        mode: 'onChange',
        defaultValues: { text: value?.kind === 'text' ? value.text : '' },
    })
    const apply = () =>
        void form.handleSubmit(submitted => onApply({ kind: 'text', text: submitted.text }))()
    return (
        <View className="gap-2">
            <TextInput
                name="text"
                control={form.control}
                label={def.label ?? 'Value'}
                testID="report-filter-value"
                onSubmitEditing={apply}
            />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function NumberBody({ def, value, onApply }: BodyProps) {
    const form = useForm<NumberValue>({
        resolver: zodResolver(numberValueSchema),
        mode: 'onChange',
        // Empty until a number is typed, so Apply never applies a number nobody chose.
        defaultValues: { number: value?.kind === 'number' ? value.number : undefined },
    })
    const apply = () =>
        void form.handleSubmit(submitted => onApply({ kind: 'number', number: submitted.number }))()
    return (
        <View className="gap-2">
            <NumberInput
                name="number"
                control={form.control}
                label={def.label ?? 'Value'}
                allowDecimal
                testID="report-filter-value"
            />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

// An amount in a currency (R29): a blank currency compares with an amount in any.
function AmountBody({ def, value, onApply }: BodyProps) {
    const current = value?.kind === 'amount' ? value : null
    const form = useForm<AmountValue>({
        resolver: zodResolver(amountValueSchema),
        mode: 'onChange',
        defaultValues: {
            amount: current ? textFromCents(current.cents) : '',
            currency: current?.currency ?? '',
        },
    })
    const apply = () => void form.handleSubmit(submitted => onApply(amountParamValue(submitted)))()
    return (
        <View className="gap-2">
            <TextInput
                name="amount"
                control={form.control}
                label={def.label ?? 'Amount'}
                keyboardType="decimal-pad"
                testID="report-filter-value"
                onSubmitEditing={apply}
            />
            <TextInput
                name="currency"
                control={form.control}
                label="Currency"
                autoCapitalize="characters"
                placeholder="Any currency"
                maxLength={3}
                testID="report-filter-currency"
                onSubmitEditing={apply}
            />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function GroupHead({
    group,
    isPicked,
    canPickAll,
    onPickAll,
}: {
    group: ParamOptionGroup | null
    isPicked: boolean
    canPickAll: boolean
    onPickAll: (value: string) => void
}) {
    if (!group) return null
    const label = optionText(group)
    return (
        <View className="flex-row items-center justify-between gap-2 px-2 pt-2">
            <Text
                numberOfLines={1}
                className="min-w-0 flex-1 text-xs font-semibold uppercase text-muted-foreground"
            >
                {label}
            </Text>
            <AllOfGroup
                isVisible={canPickAll}
                isPicked={isPicked}
                value={group.value}
                label={label}
                onPress={() => onPickAll(group.value)}
            />
        </View>
    )
}

function AllOfGroup({
    isVisible,
    isPicked,
    value,
    label,
    onPress,
}: {
    isVisible: boolean
    isPicked: boolean
    value: string
    label: string
    onPress: () => void
}) {
    if (!isVisible) return null
    const tone = isPicked ? 'border-primary bg-primary' : 'border-border bg-transparent'
    const text = isPicked ? 'text-primary-foreground' : 'text-primary'
    return (
        <Pressable
            testID={`report-filter-group-${value}`}
            accessibilityRole="checkbox"
            accessibilityState={{ checked: isPicked }}
            aria-checked={isPicked}
            onPress={onPress}
            style={TARGET_STYLE}
            className={`justify-center rounded-full border px-3 ${tone}`}
        >
            <Text className={`text-sm ${text}`}>{`All ${label}`}</Text>
        </Pressable>
    )
}

function SectionOptions({
    options,
    values,
    isGroupPicked,
    isMultiple,
    onToggle,
}: {
    options: ParamOption[]
    values: string[]
    isGroupPicked: boolean
    isMultiple: boolean
    onToggle: (value: string) => void
}) {
    const rows = options.map(option => (
        <OptionRow
            key={option.value}
            value={option.value}
            label={optionText(option)}
            isChecked={isGroupPicked || values.includes(option.value)}
            isRadio={!isMultiple}
            onPress={() => onToggle(option.value)}
        />
    ))
    return <>{rows}</>
}

function Note({
    message,
    testID,
    tone = 'muted',
}: {
    message: string | null | undefined
    testID: string
    tone?: 'muted' | 'danger'
}) {
    if (!message) return null
    const color = tone === 'danger' ? 'text-danger' : 'text-muted-foreground'
    return (
        <Text
            role={tone === 'danger' ? 'alert' : undefined}
            testID={testID}
            className={`px-2 text-xs ${color}`}
        >
            {message}
        </Text>
    )
}

function LoadingMoreOptions({ isVisible }: { isVisible: boolean }) {
    const color = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <View testID="report-filter-more" className="items-center py-2">
            <ActivityIndicator size="small" color={color} />
        </View>
    )
}

function OptionsError({ isVisible, onRetry }: { isVisible: boolean; onRetry: () => void }) {
    if (!isVisible) return null
    return (
        <View className="flex-row items-center gap-2 px-2">
            <Text role="alert" testID="report-filter-load-error" className="text-xs text-danger">
                The options could not be loaded.
            </Text>
            <Button variant="outline" size="sm" testID="report-filter-retry" onPress={onRetry}>
                <ButtonText>Retry</ButtonText>
            </Button>
        </View>
    )
}

function SourceApply({
    isVisible,
    ...props
}: {
    isVisible: boolean
    isDisabled: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return <ApplyButton {...props} />
}

interface SourceBodyProps extends BodyProps {
    source: OptionsSource
    params: Record<string, unknown>
}

const SEARCH_DELAY_MS = 250

// Options read from a source under the reader's rules: searched, loaded a page at a
// time near the end of the list, and picked one by one or a whole group at a time
// (a group reference the server expands at run time, R30).
function SourceChoiceBody({ def, value, onApply, source, params }: SourceBodyProps) {
    const chosen = value?.kind === 'choice' ? value : null
    const form = useForm<SourceChoiceValues>({
        resolver: zodResolver(sourceChoiceSchema),
        mode: 'onChange',
        defaultValues: { search: '', values: chosen?.values ?? [], groups: chosen?.groups ?? [] },
    })
    const typed = useWatch({ control: form.control, name: 'search' })
    const search = useDebouncedValue(typed, SEARCH_DELAY_MS)
    const values = useWatch({ control: form.control, name: 'values' })
    const groups = useWatch({ control: form.control, name: 'groups' })
    const options = useParamOptions({ source, param: def.id, search, params, isEnabled: true })
    const pages = options.data?.pages ?? []
    const sections = optionSections(pages)
    const isMultiple = def.multiple === true
    const error = form.formState.errors.values?.message ?? form.formState.errors.groups?.message
    const toggle = (option: string) => {
        if (!isMultiple) {
            onApply({ kind: 'choice', values: [option] })
            return
        }
        const next = values.includes(option)
            ? values.filter(v => v !== option)
            : [...values, option]
        form.setValue('values', next, { shouldValidate: true })
    }
    const pickAll = (group: string) =>
        form.setValue('groups', toggleGroup(groups, group), { shouldValidate: true })
    const onScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) => {
        if (isNearEnd(event.nativeEvent) && options.hasNextPage && !options.isFetchingNextPage) {
            void options.fetchNextPage()
        }
    }
    const body = sections.map(section => {
        const isGroupPicked = section.group !== null && groups.includes(section.group.value)
        return (
            <View key={section.group?.value ?? ':loose'}>
                <GroupHead
                    group={section.group}
                    isPicked={isGroupPicked}
                    canPickAll={isMultiple}
                    onPickAll={pickAll}
                />
                <SectionOptions
                    options={section.options}
                    values={values}
                    isGroupPicked={isGroupPicked}
                    isMultiple={isMultiple}
                    onToggle={toggle}
                />
            </View>
        )
    })
    const isEmpty = values.length === 0 && groups.length === 0
    const noMatches = options.isSuccess && sections.length === 0 ? 'No matches' : null
    // The server lists at most 200 groups (F9); a search narrows them.
    const groupsCut = pages[0]?.groupsCut
        ? 'Only the first 200 groups are listed. Search to narrow them.'
        : null
    // A value without whole groups has no groups list, as one read from a URL.
    const apply = () =>
        void form.handleSubmit(({ values: picked, groups: whole }) =>
            onApply(
                whole.length > 0
                    ? { kind: 'choice', values: picked, groups: whole }
                    : { kind: 'choice', values: picked }
            )
        )()
    return (
        <View className="gap-2">
            <TextInput
                name="search"
                control={form.control}
                label="Search"
                testID="report-filter-search"
                autoCorrect={false}
            />
            <Note message={groupsCut} testID="report-filter-groups-cut" />
            <ScrollView
                style={LIST_STYLE}
                nestedScrollEnabled
                onScroll={onScroll}
                scrollEventThrottle={16}
            >
                {body}
                <Note message={noMatches} testID="report-filter-no-matches" />
                <LoadingMoreOptions isVisible={options.isFetchingNextPage} />
            </ScrollView>
            <OptionsError isVisible={options.isError} onRetry={() => void options.refetch()} />
            <Note message={error} testID="report-filter-error" tone="danger" />
            <SourceApply
                isVisible={isMultiple}
                isDisabled={isEmpty || error !== undefined}
                onPress={apply}
            />
        </View>
    )
}

interface PickerBodyProps extends BodyProps {
    isVisible: boolean
    source: OptionsSource | null
    params: Record<string, unknown>
}

function PickerBody({ isVisible, source, params, ...props }: PickerBodyProps) {
    if (!isVisible) return null
    const { def } = props
    if (def.type === 'choice' && def.optionsFrom && source) {
        return <SourceChoiceBody {...props} source={source} params={params} />
    }
    if (def.type === 'choice' && def.multiple) return <MultiChoiceBody {...props} />
    if (def.type === 'choice') return <SingleChoiceBody {...props} />
    if (def.type === 'amount') return <AmountBody {...props} />
    if (def.type === 'number') return <NumberBody {...props} />
    return <TextBody {...props} />
}

interface ParamPickerProps extends BodyProps {
    trigger: ReactElement
    isOpen: boolean
    onOpenChange(open: boolean): void
    /** Where a choice from a source reads its options; null while the report loads. */
    source: OptionsSource | null
    /** The run's other params, which an options filter may read. */
    params: Record<string, unknown>
    title?: string
}

// The picker of a choice, text, number or amount param. Closing without Apply
// changes nothing. On a phone it is a bottom sheet.
export function ParamPicker({ trigger, isOpen, onOpenChange, title, ...body }: ParamPickerProps) {
    return (
        <Popover
            trigger={trigger}
            isOpen={isOpen}
            onOpenChange={onOpenChange}
            title={title ?? body.def.label ?? 'Filter'}
            sheetSide="bottom"
            width={PICKER_WIDTH}
            testID="report-filter-picker"
        >
            <PickerBody isVisible={isOpen} {...body} />
        </Popover>
    )
}

/** The searching record picker over a source, for the builder's record values (R19). */
export function SourcePicker(props: ParamPickerProps & { source: OptionsSource }) {
    return <ParamPicker {...props} />
}
