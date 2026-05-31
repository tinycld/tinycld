import * as fs from 'node:fs'
import * as path from 'node:path'
import { describe, expect, it } from 'vitest'
import { buildPackageIconsSource, kebabToPascal } from '../gen-icons'

const LUCIDE_PKG_ROOT = path.resolve(
    path.dirname(require.resolve('lucide-react-native')),
    '..',
    '..'
)
const LUCIDE_ICONS_DIR = path.join(LUCIDE_PKG_ROOT, 'dist', 'esm', 'icons')

describe('kebabToPascal', () => {
    it('capitalizes single-segment names', () => {
        expect(kebabToPascal('mail')).toBe('Mail')
        expect(kebabToPascal('users')).toBe('Users')
    })
    it('joins multi-segment names', () => {
        expect(kebabToPascal('hard-drive')).toBe('HardDrive')
        expect(kebabToPascal('file-text')).toBe('FileText')
        expect(kebabToPascal('cloud-rain')).toBe('CloudRain')
    })
})

describe('buildPackageIconsSource', () => {
    it('emits an import per distinct kebab name and a sorted Record', () => {
        const src = buildPackageIconsSource([
            { name: '@tinycld/mail', manifest: { nav: { icon: 'mail' } } },
            { name: '@tinycld/drive', manifest: { nav: { icon: 'hard-drive' } } },
        ])
        expect(src).toContain('Mail,')
        expect(src).toContain('HardDrive,')
        expect(src).toContain("'hard-drive': HardDrive")
        expect(src).toContain('mail: Mail')
        // sorted alphabetically: hard-drive (h) before mail (m)
        expect(src.indexOf("'hard-drive':")).toBeLessThan(src.indexOf('mail:'))
    })

    it('deduplicates icons used by multiple packages', () => {
        const src = buildPackageIconsSource([
            { name: '@tinycld/a', manifest: { nav: { icon: 'mail' } } },
            { name: '@tinycld/b', manifest: { nav: { icon: 'mail' } } },
        ])
        // One indented import line, one indented map entry.
        const importMatches = src.match(/^ {4}Mail,$/gm) ?? []
        const mapMatches = src.match(/^ {4}mail: Mail,$/gm) ?? []
        expect(importMatches.length).toBe(1)
        expect(mapMatches.length).toBe(1)
    })

    it('skips features with no nav.icon', () => {
        const src = buildPackageIconsSource([
            { name: '@tinycld/settings-only', manifest: {} },
            { name: '@tinycld/mail', manifest: { nav: { icon: 'mail' } } },
        ])
        expect(src).toContain('Mail,')
        expect(src).toContain('mail: Mail')
    })

    it('emits a valid empty module when no icons are used', () => {
        const src = buildPackageIconsSource([{ name: '@tinycld/x', manifest: {} }])
        expect(src).toContain('export const packageIcons: Record<string, LucideIcon> = {}')
    })

    it('throws on an unknown lucide name with the offending name', () => {
        expect(() =>
            buildPackageIconsSource([
                { name: '@tinycld/typo', manifest: { nav: { icon: 'not-a-real-icon' } } },
            ])
        ).toThrow(/not-a-real-icon/)
    })

    it('suggests the closest match when a name is a typo', () => {
        // 'maill' is one edit away from 'mail' — should trigger a suggestion.
        let caught: Error | null = null
        try {
            buildPackageIconsSource([
                { name: '@tinycld/typo', manifest: { nav: { icon: 'maill' } } },
            ])
        } catch (e) {
            caught = e as Error
        }
        expect(caught).not.toBeNull()
        expect(caught!.message).toMatch(/Did you mean 'mail'/)
    })

    it('names the offending package in the error', () => {
        expect(() =>
            buildPackageIconsSource([
                { name: '@tinycld/bogus', manifest: { nav: { icon: 'fake-icon-xyz' } } },
            ])
        ).toThrow(/@tinycld\/bogus/)
    })

    it('every kebab key in the emitted map corresponds to a real lucide icon file', () => {
        const src = buildPackageIconsSource([
            { name: '@tinycld/mail', manifest: { nav: { icon: 'mail' } } },
            { name: '@tinycld/drive', manifest: { nav: { icon: 'hard-drive' } } },
            { name: '@tinycld/calendar', manifest: { nav: { icon: 'calendar' } } },
        ])
        for (const name of ['mail', 'hard-drive', 'calendar']) {
            expect(fs.existsSync(path.join(LUCIDE_ICONS_DIR, `${name}.mjs`))).toBe(true)
            expect(src).toContain(name)
        }
    })
})
