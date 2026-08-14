// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { Text } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'
import { useWarmEditor } from '../use-warm-editor.web'
import { WarmEditorHost } from '../WarmEditorHost.web'

/**
 * Web has no WebView and no cold start, so there is nothing to warm. The host
 * still renders its children and the hook still answers — reporting isWarm
 * false so the consumer mounts its own editor. That keeps LazyEditor's call
 * sites identical on both platforms.
 */
function Probe() {
    const lease = useWarmEditor('composer:card1', {})
    return <Text>{lease.isWarm ? 'warm' : 'cold'}</Text>
}

describe('warm editor on web', () => {
    // render() appends to the shared document.body, so without this a later
    // query matches the previous test's tree as well as its own.
    afterEach(cleanup)

    it('renders children rather than swallowing the tree', () => {
        const { getByText } = render(
            <WarmEditorHost options={{}}>
                <Text>content</Text>
            </WarmEditorHost>
        )
        expect(getByText('content')).toBeTruthy()
    })

    it('reports cold, so consumers mount their own editor', () => {
        const { getByText } = render(
            <WarmEditorHost options={{}}>
                <Probe />
            </WarmEditorHost>
        )
        expect(getByText('cold')).toBeTruthy()
    })

    it('reports cold with no host mounted at all', () => {
        const { getByText } = render(<Probe />)
        expect(getByText('cold')).toBeTruthy()
    })
})
