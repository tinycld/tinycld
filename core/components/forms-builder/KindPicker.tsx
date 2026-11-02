import type { CustomKind } from '@tinycld/core/lib/forms/definition'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Menu } from '@tinycld/core/ui/menu'

interface KindGroup {
    label: string
    kinds: { kind: CustomKind; label: string }[]
}

const GROUPS: KindGroup[] = [
    {
        label: 'Text',
        kinds: [
            { kind: 'text', label: 'Text' },
            { kind: 'textarea', label: 'Long text' },
            { kind: 'markdown', label: 'Markdown' },
            { kind: 'richtext', label: 'Rich text' },
        ],
    },
    {
        label: 'Choices',
        kinds: [
            { kind: 'select', label: 'Choice list' },
            { kind: 'toggle', label: 'Toggle' },
        ],
    },
    { label: 'People', kinds: [{ kind: 'user', label: 'Person' }] },
    {
        label: 'Other',
        kinds: [
            { kind: 'number', label: 'Number' },
            { kind: 'date', label: 'Date' },
            { kind: 'checklist', label: 'Checklist' },
            { kind: 'file', label: 'File' },
        ],
    },
    {
        label: 'Tables and totals',
        kinds: [
            { kind: 'table', label: 'Table' },
            { kind: 'computed', label: 'Calculated value' },
        ],
    },
]

export interface KindPickerProps {
    onPick: (kind: CustomKind) => void
    /** The kinds the host allows; absent offers every kind. */
    kinds?: readonly CustomKind[]
    testID?: string
}

function KindSection({ group, onPick }: { group: KindGroup; onPick: KindPickerProps['onPick'] }) {
    const items = group.kinds.map(k => (
        <Menu.Item
            key={k.kind}
            label={k.label}
            onSelect={() => onPick(k.kind)}
            testID={`form-kind-${k.kind}`}
        />
    ))
    return <Menu.Section label={group.label}>{items}</Menu.Section>
}

function allowedGroups(kinds: readonly CustomKind[] | undefined): KindGroup[] {
    if (!kinds) return GROUPS
    return GROUPS.map(g => ({ ...g, kinds: g.kinds.filter(k => kinds.includes(k.kind)) })).filter(
        g => g.kinds.length > 0
    )
}

export function KindPicker({ onPick, kinds, testID = 'form-add-field' }: KindPickerProps) {
    const trigger = (
        <Button variant="outline" size="sm" className="self-start mt-2" testID={testID}>
            <ButtonText>Add field</ButtonText>
        </Button>
    )
    const sections = allowedGroups(kinds).map(g => (
        <KindSection key={g.label} group={g} onPick={onPick} />
    ))
    return (
        <Menu presentation="auto" title="Add a field" trigger={trigger}>
            {sections}
        </Menu>
    )
}
