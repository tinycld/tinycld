import { describe, expect, it } from 'vitest'
import { orderGroupsByUserPreference } from '../../lib/automation/condition-helpers'

// The trigger and action menus group by package. They used to come out
// alphabetically by slug (a stability hack in use-automation-catalog), which
// matched nothing else in the app. Now they follow the order the user dragged
// their apps into in Settings → Personal → Navigation, so the menus read like
// the sidebar and tab bar.

function groupsOf(...slugs: string[]) {
    return new Map(slugs.map(slug => [slug, [`${slug}-item`]]))
}

function orderedSlugsOf(result: [string, string[]][]) {
    return result.map(([slug]) => slug)
}

describe('orderGroupsByUserPreference', () => {
    it('follows the user order rather than the alphabet', () => {
        const result = orderGroupsByUserPreference(groupsOf('calendar', 'mail', 'drive'), [
            'mail',
            'drive',
            'calendar',
        ])
        expect(orderedSlugsOf(result)).toEqual(['mail', 'drive', 'calendar'])
    })

    it('puts core first, ahead of everything the user ordered', () => {
        // core owns the synthetic run-manually / on-a-schedule triggers — the
        // package-neutral starting points every rule can use — and it has no
        // manifest, so it never appears in the nav order to be placed by hand.
        const result = orderGroupsByUserPreference(groupsOf('mail', 'core', 'drive'), [
            'mail',
            'drive',
        ])
        expect(orderedSlugsOf(result)[0]).toBe('core')
    })

    it('keeps core first even when the user order happens to name it', () => {
        const result = orderGroupsByUserPreference(groupsOf('mail', 'core'), ['mail', 'core'])
        expect(orderedSlugsOf(result)).toEqual(['core', 'mail'])
    })

    it('appends packages missing from the nav order instead of dropping them', () => {
        // Settings-only contributors have no nav entry, so useSortedPackages
        // filters them out — but they can still contribute automation, and a
        // menu that silently omitted their triggers would be worse than one
        // that lists them last.
        const result = orderGroupsByUserPreference(groupsOf('takeout', 'mail'), ['mail'])
        expect(orderedSlugsOf(result)).toEqual(['mail', 'takeout'])
    })

    it('sorts unranked packages alphabetically so the menu is stable', () => {
        // Map insertion order follows the catalog's row order, which is not a
        // guarantee worth depending on for what the user sees.
        const result = orderGroupsByUserPreference(groupsOf('zeta', 'alpha', 'mid'), [])
        expect(orderedSlugsOf(result)).toEqual(['alpha', 'mid', 'zeta'])
    })

    it('falls back to alphabetical-after-core when no preference is set', () => {
        const result = orderGroupsByUserPreference(groupsOf('mail', 'core', 'calendar'), [])
        expect(orderedSlugsOf(result)).toEqual(['core', 'calendar', 'mail'])
    })

    it('leaves the grouped items untouched', () => {
        const result = orderGroupsByUserPreference(groupsOf('mail'), ['mail'])
        expect(result).toEqual([['mail', ['mail-item']]])
    })

    it('handles an empty catalog', () => {
        expect(orderGroupsByUserPreference(new Map(), ['mail'])).toEqual([])
    })
})
