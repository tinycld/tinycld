import type { RuleDraft } from '@tinycld/core/lib/automation/draft'
import { WEB_FORM_HOST_KEY } from '@tinycld/core/lib/forms/core-hosts'
import { useFormsOfHost } from '@tinycld/core/lib/forms/use-forms-of-host'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { type Control, Controller } from '@tinycld/core/ui/form'
import { Menu } from '@tinycld/core/ui/menu'
import { Text, View } from 'react-native'

type WebFormOption = { id: string; name: string }

function useWebForms(): WebFormOption[] {
    const { forms } = useFormsOfHost(WEB_FORM_HOST_KEY)
    return forms.filter(f => !f.archived).map(f => ({ id: f.id, name: f.name }))
}

function FormMenu({
    forms,
    value,
    onChange,
}: {
    forms: WebFormOption[]
    value: string | undefined
    onChange: (id: string) => void
}) {
    const current = forms.find(f => f.id === value)?.name ?? 'Pick a form'
    const items = forms.map(f => (
        <Menu.Item
            key={f.id}
            label={f.name}
            isSelected={f.id === value}
            onSelect={() => onChange(f.id)}
            testID={`rule-form-picker-${f.id}`}
        />
    ))
    const trigger = (
        <Button variant="outline" size="sm" className="self-start" testID="rule-form-picker">
            <ButtonText>{current}</ButtonText>
        </Button>
    )
    return (
        <Menu presentation="auto" title="Pick a form" trigger={trigger}>
            {items}
        </Menu>
    )
}

/** The web form whose submissions a core:form-submitted rule reads. */
export function FormPickerRow({
    control,
    isVisible,
}: {
    control: Control<RuleDraft>
    isVisible: boolean
}) {
    const forms = useWebForms()
    if (!isVisible) return null
    return (
        <Controller
            control={control}
            name="triggerConfig.form"
            render={({ field, fieldState }) => (
                <View className="gap-1">
                    <Text className="text-sm font-semibold text-foreground">Form</Text>
                    <FormMenu forms={forms} value={field.value} onChange={field.onChange} />
                    <Text className="text-xs text-danger">{fieldState.error?.message ?? ''}</Text>
                </View>
            )}
        />
    )
}
