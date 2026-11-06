import { type BrandColorValues, brandColorSchema, swatchColor } from '@tinycld/core/lib/brand-color'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useOrgBranding } from '@tinycld/core/lib/use-org-branding'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { COLOR_PALETTE, ColorPickerGrid } from '@tinycld/core/ui/color-picker'
import { TextInput, useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'
import { Text, View } from 'react-native'

const HINT =
    'Used on documents the apps print, such as PDF files: the heading, the rules and the table header. Leave empty to use the default.'

// The form edits the one branding row. When no row exists yet (no logo was ever
// uploaded) the first save creates it, as the first logo upload does.
function useBrandColorEditor() {
    const { branding, brandingCollection } = useOrgBranding()
    const form = useForm<BrandColorValues>({
        resolver: zodResolver(brandColorSchema),
        values: { brand_color: branding?.brand_color ?? '' },
        resetOptions: { keepDirtyValues: true },
    })
    const { control, setValue, setError, getValues, handleSubmit, formState } = form
    const value = useWatch({ control, name: 'brand_color' })

    const save = useMutation({
        mutationFn: mutation(function* (brandColor: string) {
            if (branding) {
                yield brandingCollection.update(branding.id, draft => {
                    draft.brand_color = brandColor
                })
                return
            }
            yield brandingCollection.insert({
                id: newRecordId(),
                logo: '',
                logo_crop: '',
                brand_color: brandColor,
            })
        }),
        onError: handleMutationErrorsWithForm({
            setError,
            getValues,
            operation: 'settings.brand_color',
        }),
    })

    const pick = (hex: string) =>
        setValue('brand_color', hex, { shouldDirty: true, shouldValidate: true })
    const submit = () => void handleSubmit(values => save.mutate(values.brand_color))()

    return {
        control,
        value,
        swatch: swatchColor(value),
        pick,
        submit,
        canSave: formState.isDirty && !save.isPending,
    }
}

export function BrandColorSection({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <BrandColorEditor />
}

function BrandColorEditor() {
    const editor = useBrandColorEditor()
    return (
        <View className="gap-3">
            <Text className="text-foreground text-xl font-bold">Brand color</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-3">
                <Text className="text-[13px] text-muted-foreground">{HINT}</Text>
                <View className="flex-row items-center gap-3">
                    <View
                        testID="brand-color-swatch"
                        className="h-9 w-9 rounded-lg border border-border"
                        style={{ backgroundColor: editor.swatch }}
                    />
                    <View className="flex-1">
                        <TextInput
                            control={editor.control}
                            name="brand_color"
                            label="Color"
                            placeholder="#0d9488"
                            autoCapitalize="none"
                            autoCorrect={false}
                            testID="brand-color-input"
                        />
                    </View>
                </View>
                <ColorPickerGrid
                    selected={editor.value}
                    onSelect={editor.pick}
                    palette={COLOR_PALETTE}
                    showClear
                    clearLabel="Use default"
                />
                <View className="flex-row">
                    <Button
                        testID="brand-color-save"
                        isDisabled={!editor.canSave}
                        onPress={editor.submit}
                    >
                        <ButtonText>Save</ButtonText>
                    </Button>
                </View>
            </View>
        </View>
    )
}
