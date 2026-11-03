import { RuleBuilder } from '@tinycld/core/components/rules/RuleBuilder'
import { RuleRow } from '@tinycld/core/components/rules/RuleRow'
import { RunHistory } from '@tinycld/core/components/rules/RunHistory'
import { FORM_SUBMITTED_TRIGGER } from '@tinycld/core/lib/automation/form-trigger'
import { useAutomationCatalog } from '@tinycld/core/lib/automation/use-automation-catalog'
import { useRulesUiStore } from '@tinycld/core/lib/stores/rules-ui-store'
import { useDestinations } from '@tinycld/core/lib/web-forms/use-destinations'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Text, View } from 'react-native'

function useDestinationsSection(formId: string, formName: string) {
    const d = useDestinations(formId)
    const { catalog } = useAutomationCatalog()
    const builder = useRulesUiStore(s => s.builder)
    const openCreate = useRulesUiStore(s => s.openCreate)
    const closeBuilder = useRulesUiStore(s => s.closeBuilder)
    const historyRuleId = useRulesUiStore(s => s.historyRuleId)
    const closeHistory = useRulesUiStore(s => s.closeHistory)
    return {
        ...d,
        catalog,
        closeBuilder,
        historyRuleId,
        closeHistory,
        isBuilderOpen: builder.mode !== 'closed',
        ruleId: builder.mode === 'edit' ? builder.ruleId : undefined,
        preset: builder.mode === 'create' ? builder.preset : undefined,
        nextOrder: builder.mode === 'create' ? builder.nextOrder : 0,
        add: () =>
            openCreate('org', d.nextOrder, undefined, {
                trigger: FORM_SUBMITTED_TRIGGER,
                triggerConfig: { form: formId },
                name: `${formName}: `,
            }),
    }
}

type Section = ReturnType<typeof useDestinationsSection>

function DestinationRows({ s }: { s: Section }) {
    const catalog = s.catalog
    if (!catalog) return null
    const rows = s.rules.map(rule => (
        <RuleRow
            key={rule.id}
            rule={rule}
            catalog={catalog}
            canEdit
            lastRun={s.lastRunByRule.get(rule.id)}
        />
    ))
    return <View>{rows}</View>
}

/** The org rules each submission of this form is given to, and a way to add one. */
export function DestinationsSection({ formId, formName }: { formId: string; formName: string }) {
    const s = useDestinationsSection(formId, formName)
    return (
        <View className="gap-2" testID="web-form-destinations">
            <Text className="text-foreground font-medium">Destinations</Text>
            <Text className="text-muted-foreground text-sm">
                Each destination is an organization rule that runs for every submission.
            </Text>
            <DestinationRows s={s} />
            <Button
                size="sm"
                variant="outline"
                className="self-start"
                onPress={s.add}
                testID="web-form-add-destination"
            >
                <ButtonText>Add destination</ButtonText>
            </Button>
            <RuleBuilder
                isOpen={s.isBuilderOpen}
                onClose={s.closeBuilder}
                scope="org"
                ruleId={s.ruleId}
                preset={s.preset}
                nextOrder={s.nextOrder}
            />
            <RunHistory ruleId={s.historyRuleId} onClose={s.closeHistory} />
        </View>
    )
}
