import { describe, expect, it } from 'vitest'
import { progressActionFor, titleFor } from '../InstallProgressModal'

// The progress panel is shared by every background package job. Its title used
// to be hard-coded to "Installing…", so an uninstall or a revert read as an
// install for its whole run.
describe('titleFor', () => {
    it('names the action the panel is tracking', () => {
        expect(titleFor('install', 'running')).toBe('Installing Package...')
        expect(titleFor('uninstall', 'running')).toBe('Uninstalling Package...')
        expect(titleFor('apply', 'running')).toBe('Applying Version Changes...')
        expect(titleFor('revert', 'running')).toBe('Reverting Build...')
    })

    it('keeps the install titles e2e asserts on', () => {
        expect(titleFor('install', 'success')).toBe('Installation Complete')
        expect(titleFor('install', 'failed')).toBe('Installation Failed')
    })

    it('reports the terminal state per action', () => {
        expect(titleFor('uninstall', 'success')).toBe('Uninstall Complete')
        expect(titleFor('uninstall', 'failed')).toBe('Uninstall Failed')
        expect(titleFor('revert', 'failed')).toBe('Revert Failed')
    })
})

describe('progressActionFor', () => {
    it('maps the install-log vocabulary onto the panel actions', () => {
        expect(progressActionFor('install')).toBe('install')
        expect(progressActionFor('uninstall')).toBe('uninstall')
        expect(progressActionFor('version_change')).toBe('apply')
        expect(progressActionFor('revert')).toBe('revert')
        expect(progressActionFor('enable')).toBe('install')
    })
})
