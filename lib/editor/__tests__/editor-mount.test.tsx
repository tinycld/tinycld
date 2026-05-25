// core/lib/editor/__tests__/editor-mount.test.tsx
// @vitest-environment happy-dom
import { render } from '@testing-library/react'
import { expect, test } from 'vitest'
import { Text } from 'react-native'
import { EditorMountProvider, useEditorMount, type EditorMount } from '../editor-mount'

const sample: EditorMount = {
    itemId: 'it1',
    itemName: 'Doc',
    itemFile: '',
    mimeType: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    identity: { kind: 'member', userId: 'u1', userOrgId: 'uo1', displayName: 'Ada', color: '#abc' },
    role: 'editor',
    capabilities: { canEdit: true, canComment: true, canUseFileActions: true, canMention: true },
    realtimeCredential: { kind: 'auth' },
}

function Probe() {
    const m = useEditorMount()
    return <Text>{`${m.identity.displayName}:${m.role}:${m.capabilities.canEdit}`}</Text>
}

test('provider exposes the mount to consumers', () => {
    const { getByText } = render(
        <EditorMountProvider value={sample}>
            <Probe />
        </EditorMountProvider>
    )
    expect(getByText('Ada:editor:true')).toBeTruthy()
})

test('useEditorMount throws outside a provider', () => {
    const orig = console.error
    console.error = () => {}
    expect(() => render(<Probe />)).toThrow(/EditorMountProvider/)
    console.error = orig
})
