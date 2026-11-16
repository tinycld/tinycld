import { expect, type Page, test } from '@playwright/test'
import { login } from './helpers'

/**
 * A popover surface that is not yet placed must not be visible in any frame.
 *
 * `OpenPopoverLayer` draws the surface before it knows its content size, at a
 * provisional place, and hid it only with `opacity: 0`. An element with
 * opacity 0 is still laid out, takes clicks and is visible to Playwright and
 * screen readers, so in that frame a click reached an invisible menu at the
 * wrong place, and a wait for a visible item returned before the menu was
 * placed.
 *
 * The menu is core's User menu in the package rail (a core `Menu`, placement
 * `top-start`): a cheap open with no data of its own. A new file rather than a
 * test in menu-pointer-focus.spec.ts, whose setup builds labels and a rule.
 *
 * The observer is deterministic: the first render of `OpenPopoverLayer` is
 * always unplaced, because `size` is null until the content's ResizeObserver
 * fires, and a ResizeObserver fires only in a later rendering step. A
 * MutationObserver callback is a microtask after the DOM change, so it always
 * sees the surface as React first inserts it, before that size arrives.
 */

const VIEWPORT = { width: 1280, height: 800 }

const SECOND_PORT = Number(process.env.E2E_PORT_2 ?? 7201)
const SECOND_SERVER = `localhost:${SECOND_PORT}`

interface SurfaceState {
    // A submenu panel is a `role="menu"` inside the root surface.
    isSub: boolean
    opacity: string
    visibility: string
    rect: { x: number; y: number; width: number; height: number }
    // Points inside the surface's box at that moment, and whether
    // `document.elementFromPoint` there hit the surface.
    probes: { x: number; y: number; hitsSurface: boolean }[]
}

declare global {
    interface Window {
        __surfaceStates?: SurfaceState[]
        __surfaceObserver?: MutationObserver
    }
}

function installSurfaceObserver(page: Page) {
    return page.evaluate(() => {
        const states: SurfaceState[] = []
        window.__surfaceStates = states
        const record = (el: Element) => {
            const style = getComputedStyle(el)
            const r = el.getBoundingClientRect()
            const probes: SurfaceState['probes'] = []
            for (const fx of [0.25, 0.5, 0.75]) {
                for (const fy of [0.1, 0.5, 0.9]) {
                    const x = Math.round(r.left + r.width * fx)
                    const y = Math.round(r.top + r.height * fy)
                    if (x < 0 || y < 0 || x >= innerWidth || y >= innerHeight) continue
                    const hit = document.elementFromPoint(x, y)
                    probes.push({ x, y, hitsSurface: !!hit && el.contains(hit) })
                }
            }
            states.push({
                isSub: !!el.parentElement?.closest('[role="menu"]'),
                opacity: style.opacity,
                visibility: style.visibility,
                rect: { x: r.left, y: r.top, width: r.width, height: r.height },
                probes,
            })
        }
        const observer = new MutationObserver(mutations => {
            const seen = new Set<Element>()
            for (const m of mutations) {
                if (m.type === 'attributes') {
                    const target = m.target as Element
                    if (target.getAttribute('role') === 'menu') seen.add(target)
                    continue
                }
                for (const node of m.addedNodes) {
                    if (!(node instanceof Element)) continue
                    if (node.getAttribute('role') === 'menu') seen.add(node)
                    for (const el of node.querySelectorAll('[role="menu"]')) seen.add(el)
                }
            }
            for (const el of seen) record(el)
        })
        observer.observe(document.body, {
            subtree: true,
            childList: true,
            attributes: true,
            attributeFilter: ['style', 'class'],
        })
        window.__surfaceObserver = observer
    })
}

function menuBox(page: Page) {
    return page.evaluate(() => {
        const el = document.querySelector('[role="menu"]')
        if (!el) return null
        const r = el.getBoundingClientRect()
        return { x: r.left, y: r.top, width: r.width, height: r.height }
    })
}

test.describe('popover placement: an opening menu is never visible unplaced', () => {
    test('the User menu shows only once placed, takes no hit while unplaced, and its first visible box is its box at rest', async ({
        page,
    }) => {
        await login(page)
        await page.setViewportSize(VIEWPORT)
        const trigger = page.getByLabel('User menu', { exact: true })
        await expect(trigger).toBeVisible()

        await installSurfaceObserver(page)
        await trigger.click()
        await expect(page.getByRole('menuitem', { name: 'Sign out', exact: true })).toBeVisible()
        const firstVisibleBox = await menuBox(page)

        let lastBox = ''
        await expect
            .poll(async () => {
                const box = JSON.stringify(await menuBox(page))
                const isStable = box === lastBox && box !== 'null'
                lastBox = box
                return isStable
            })
            .toBe(true)
        const restBox = JSON.parse(lastBox) as NonNullable<Awaited<ReturnType<typeof menuBox>>>

        const states = await page.evaluate(() => {
            window.__surfaceObserver?.disconnect()
            return window.__surfaceStates ?? []
        })

        // The first render is always unplaced, so the observer sees one.
        const unplaced = states.filter(s => Number.parseFloat(s.opacity) === 0)
        expect(unplaced.length).toBeGreaterThan(0)

        // Where only the unplaced surface sits, nothing of the menu takes a hit.
        // The unplaced surface has no size yet, so it is not where the User
        // menu rests, above its trigger: with the hiding removed, a run
        // recorded hits at y 765 (x 66-166), below the rest box. Some probes
        // must fall outside the rest box, or the check below would pass with
        // nothing checked.
        const outsideRest = (p: { x: number; y: number }) =>
            p.x < restBox.x ||
            p.x > restBox.x + restBox.width ||
            p.y < restBox.y ||
            p.y > restBox.y + restBox.height
        const strayProbes = unplaced.flatMap(s => s.probes.filter(outsideRest))
        expect(strayProbes.length).toBeGreaterThan(0)
        expect(strayProbes.filter(p => p.hitsSurface)).toEqual([])

        expect(unplaced.map(s => s.visibility)).toEqual(unplaced.map(() => 'hidden'))

        // A wait for a visible item, as a user waits, reads the placed menu.
        expect(firstVisibleBox).toEqual(restBox)
    })
})

test.describe('popover placement: keyboard focus once the menu shows', () => {
    // The surface opens hidden (`visibility: hidden` on web), where focus()
    // does nothing, so the open's focus-in must land again once it shows.
    test('Enter on the User menu focuses its first item, arrows move, Escape returns to the trigger', async ({
        page,
    }) => {
        await login(page)
        await page.setViewportSize(VIEWPORT)
        const trigger = page.getByLabel('User menu', { exact: true })
        await expect(trigger).toBeVisible()

        await trigger.focus()
        await page.keyboard.press('Enter')
        const menu = page.getByRole('menu')
        const settings = menu.getByRole('menuitem', { name: 'Settings', exact: true })
        await expect(settings).toBeVisible()
        await expect(settings).toBeFocused()

        await page.keyboard.press('ArrowDown')
        await expect(menu.getByRole('menuitem', { name: 'Sign out', exact: true })).toBeFocused()

        await page.keyboard.press('Escape')
        await expect(menu).toHaveCount(0)
        await expect(trigger).toBeFocused()
    })

    // The User menu draws its "Remove server" submenu only with two saved
    // servers. The saved-server list is this browser's own storage, not
    // server data; it is seeded before the app boots, in the shape the app
    // writes, as server-switcher.spec.ts does. Adding a server through
    // Settings would navigate to the other origin.
    test('a submenu opened from the keyboard focuses its first row once shown and is never visible unplaced', async ({
        page,
    }) => {
        await page.addInitScript(second => {
            const own = window.location.origin
            const raw = window.localStorage.getItem('tinycld:servers')
            const list: { origin: string; label: string; addedAt: number }[] = raw
                ? JSON.parse(raw)
                : []
            for (const origin of [own, second]) {
                if (list.some(s => s.origin === origin)) continue
                list.push({ origin, label: new URL(origin).host, addedAt: Date.now() })
            }
            window.localStorage.setItem('tinycld:servers', JSON.stringify(list))
        }, `http://${SECOND_SERVER}`)
        await login(page)
        await page.setViewportSize(VIEWPORT)
        const trigger = page.getByLabel('User menu', { exact: true })
        await expect(trigger).toBeVisible()

        await installSurfaceObserver(page)
        await trigger.focus()
        await page.keyboard.press('Enter')
        const removeRow = page.getByRole('menuitem', { name: 'Remove server', exact: true })
        await expect(removeRow).toBeVisible()
        await page.keyboard.press('End')
        await page.keyboard.press('ArrowUp')
        await expect(removeRow).toBeFocused()

        const subRow = page
            .locator('[role="menu"] [role="menu"]')
            .getByRole('menuitem', { name: SECOND_SERVER, exact: true })
        await page.keyboard.press('Enter')
        await expect(subRow).toBeVisible()
        await expect(subRow).toBeFocused()

        await page.keyboard.press('ArrowLeft')
        await expect(subRow).toHaveCount(0)
        await expect(removeRow).toBeFocused()
        await page.keyboard.press('ArrowRight')
        await expect(subRow).toBeVisible()
        await expect(subRow).toBeFocused()

        const states = await page.evaluate(() => {
            window.__surfaceObserver?.disconnect()
            return window.__surfaceStates ?? []
        })
        // Each open mounts the panel unplaced (its size is unknown until
        // onLayout), so the observer sees it hidden at least once.
        const subUnplaced = states.filter(s => s.isSub && Number.parseFloat(s.opacity) === 0)
        expect(subUnplaced.length).toBeGreaterThan(0)
        expect(subUnplaced.map(s => s.visibility)).toEqual(subUnplaced.map(() => 'hidden'))
    })
})
