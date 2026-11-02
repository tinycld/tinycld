import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { TextInput, useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
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
import { type SourceChoiceValues, sourceChoiceSchema } from '../lib/param-forms'
import {
    hasOptionGroups,
    type OptionSection,
    optionSections,
    optionText,
    toggleGroup,
} from '../lib/param-options'
import type { ParamValue } from '../lib/params'
import type { ParamOption } from '../lib/types'
import {
    ApplyButton,
    type BodyProps,
    LIST_STYLE,
    OptionRow,
    TARGET_STYLE,
} from './ParamPickerParts'

// The picker body of a choice param whose options come from a source (E8).

/** About ten options before the end, so the next page is there before the reader is. */
const NEAR_END_PX = 600

function isNearEnd({ contentOffset, layoutMeasurement, contentSize }: NativeScrollEvent) {
    return contentOffset.y + layoutMeasurement.height >= contentSize.height - NEAR_END_PX
}

const SEARCH_DELAY_MS = 250
const OTHER_GROUPS_LABEL = 'Other'
const GROUPS_CUT_MESSAGE = 'Only the first 200 groups are listed. Search to narrow them.'

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

// A group's head, with "All <group>" on a multiple choice. The options of groups
// the server did not list share an Other head, which cannot be picked whole: its
// groups have no label to show on the chip.
function SectionHead({
    section,
    isPicked,
    canPickAll,
    onPickAll,
}: {
    section: OptionSection
    isPicked: boolean
    canPickAll: boolean
    onPickAll: (value: string) => void
}) {
    const { group } = section
    if (!group && !section.isOther) return null
    const label = group ? optionText(group) : OTHER_GROUPS_LABEL
    return (
        <View className="flex-row items-center justify-between gap-2 px-2 pt-2">
            <Text
                numberOfLines={1}
                className="min-w-0 flex-1 text-xs font-semibold uppercase text-muted-foreground"
            >
                {label}
            </Text>
            <AllOfGroup
                isVisible={canPickAll && group !== null}
                isPicked={isPicked}
                value={group?.value ?? ''}
                label={label}
                onPress={() => onPickAll(group?.value ?? '')}
            />
        </View>
    )
}

// Options of a group picked whole are all in it: they show checked and are not
// toggled one by one until the group is unpicked.
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
            isDisabled={isGroupPicked}
            onPress={() => onToggle(option.value)}
        />
    ))
    return <>{rows}</>
}

function Note({ message, testID }: { message: string | null | undefined; testID: string }) {
    if (!message) return null
    return (
        <Text testID={testID} className="px-2 text-xs text-muted-foreground">
            {message}
        </Text>
    )
}

function ErrorNote({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <Text role="alert" testID="report-filter-error" className="px-2 text-xs text-danger">
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

// A value without whole groups has no groups list, as one read from a URL.
function choiceOf({ values, groups }: SourceChoiceValues): ParamValue {
    if (groups.length === 0) return { kind: 'choice', values }
    return { kind: 'choice', values, groups }
}

// Options read from a source under the reader's rules: searched, loaded a page at a
// time near the end of the list (or at once while the list is shorter than its box),
// and picked one by one or a whole group at a time (a group reference the server
// expands at run time, R30).
export function SourceChoiceBody({ def, value, onApply, source, params }: SourceBodyProps) {
    const chosen = value?.kind === 'choice' ? value : null
    const form = useForm<SourceChoiceValues>({
        resolver: zodResolver(sourceChoiceSchema),
        mode: 'onChange',
        defaultValues: { search: '', values: chosen?.values ?? [], groups: chosen?.groups ?? [] },
    })
    const search = useDebouncedValue(
        useWatch({ control: form.control, name: 'search' }),
        SEARCH_DELAY_MS
    )
    // A search the form refuses (too long) is not sent; the last answer stays.
    const isSearchValid = sourceChoiceSchema.shape.search.safeParse(search).success
    const values = useWatch({ control: form.control, name: 'values' })
    const groups = useWatch({ control: form.control, name: 'groups' })
    const options = useParamOptions({
        source,
        param: def.id,
        search,
        params,
        isEnabled: isSearchValid,
    })
    const pages = options.data?.pages ?? []
    const sections = optionSections(pages, hasOptionGroups(def))
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
    const loadMore = () => {
        if (options.hasNextPage && !options.isFetchingNextPage) void options.fetchNextPage()
    }
    const onScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) => {
        if (isNearEnd(event.nativeEvent)) loadMore()
    }
    // A page that does not fill the list leaves nothing to scroll, so the next one
    // loads at once.
    const onContentSizeChange = (_width: number, height: number) => {
        if (height < LIST_STYLE.maxHeight) loadMore()
    }
    const body = sections.map(section => {
        const isGroupPicked = section.group !== null && groups.includes(section.group.value)
        return (
            <View key={section.isOther ? ':other' : `g:${section.group?.value ?? ''}`}>
                <SectionHead
                    section={section}
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
    const groupsCut = pages[0]?.groupsCut ? GROUPS_CUT_MESSAGE : null
    const apply = () => void form.handleSubmit(submitted => onApply(choiceOf(submitted)))()
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
                testID="report-filter-options"
                style={LIST_STYLE}
                nestedScrollEnabled
                onScroll={onScroll}
                onContentSizeChange={onContentSizeChange}
                scrollEventThrottle={16}
            >
                {body}
                <Note message={noMatches} testID="report-filter-no-matches" />
                <LoadingMoreOptions isVisible={options.isFetchingNextPage} />
            </ScrollView>
            <OptionsError isVisible={options.isError} onRetry={() => void options.refetch()} />
            <ErrorNote message={error} />
            <SourceApply
                isVisible={isMultiple}
                isDisabled={isEmpty || error !== undefined}
                onPress={apply}
            />
        </View>
    )
}
