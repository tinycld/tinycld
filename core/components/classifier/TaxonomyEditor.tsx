import {
    MAX_TAXONOMY_LABELS,
    type TaxonomyFormValues,
    type TaxonomyPreset,
    taxonomySchema,
} from '@tinycld/core/lib/classifier/taxonomy-schema'
import { type TaxonomyScope, useTaxonomy } from '@tinycld/core/lib/classifier/use-taxonomy'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import {
    FormErrorSummary,
    SelectInput,
    useFieldArray,
    useForm,
    zodResolver,
} from '@tinycld/core/ui/form'
import { useMemo } from 'react'
import { View } from 'react-native'
import { TaxonomyRow } from './TaxonomyRow'

interface TaxonomyEditorProps {
    app: string
    scope: TaxonomyScope
    presets: TaxonomyPreset[]
    /** Names the per-row switch, e.g. "Archive when matched". */
    autoActionLabel: string
    isVisible: boolean
    testID: string
}

const EMPTY_ROW = { name: '', assignWhen: '', autoAction: false }

function useTaxonomyEditor(app: string, scope: TaxonomyScope, presets: TaxonomyPreset[]) {
    const { rows, isReady, save } = useTaxonomy(app, scope)
    const values = useMemo(
        () => ({
            rows: rows.map(({ name, assignWhen, autoAction }) => ({
                name,
                assignWhen,
                autoAction,
            })),
        }),
        [rows]
    )
    // `values` re-syncs the form when the stored taxonomy changes.
    const {
        control,
        handleSubmit,
        formState: { errors, isSubmitted },
    } = useForm<TaxonomyFormValues>({ resolver: zodResolver(taxonomySchema), values })
    const { fields, append, remove, move, replace } = useFieldArray({ control, name: 'rows' })

    // The preset picker is not part of the saved taxonomy, so it has its own form.
    const { control: presetControl } = useForm({ defaultValues: { preset: '' } })
    const presetOptions = presets.map(p => ({ value: p.id, label: p.label }))
    const applyPreset = (id: string) => {
        const preset = presets.find(p => p.id === id)
        if (preset) replace(preset.rows)
    }
    const addRow = () => append(EMPTY_ROW)
    const submit = () => handleSubmit(next => save.mutate(next))()

    return {
        control,
        presetControl,
        fields,
        errors,
        isSubmitted,
        isReady,
        isSaving: save.isPending,
        canAdd: fields.length < MAX_TAXONOMY_LABELS,
        presetOptions,
        applyPreset,
        addRow,
        remove,
        move,
        submit,
    }
}

export function TaxonomyEditor({
    app,
    scope,
    presets,
    autoActionLabel,
    isVisible,
    testID,
}: TaxonomyEditorProps) {
    const editor = useTaxonomyEditor(app, scope, presets)
    if (!isVisible) return null

    const lastIndex = editor.fields.length - 1
    const rowList = editor.fields.map((field, index) => (
        <TaxonomyRow
            key={field.id}
            control={editor.control}
            index={index}
            isFirst={index === 0}
            isLast={index === lastIndex}
            autoActionLabel={autoActionLabel}
            onRemove={editor.remove}
            onMove={editor.move}
            testID={testID}
        />
    ))

    return (
        <View className="gap-3" testID={testID}>
            <SelectInput
                control={editor.presetControl}
                name="preset"
                label="Start from a preset"
                placeholder="Choose a preset"
                options={editor.presetOptions}
                onValueChange={editor.applyPreset}
                testID={`${testID}-preset`}
            />
            {rowList}
            <FormErrorSummary errors={editor.errors} isEnabled={editor.isSubmitted} />
            <View className="flex-row gap-2">
                <Button
                    variant="outline"
                    onPress={editor.addRow}
                    disabled={!editor.canAdd}
                    testID={`${testID}-add`}
                >
                    <ButtonText>Add label</ButtonText>
                </Button>
                <Button
                    onPress={editor.submit}
                    disabled={!editor.isReady || editor.isSaving}
                    testID={`${testID}-save`}
                >
                    <ButtonText>Save</ButtonText>
                </Button>
            </View>
        </View>
    )
}
