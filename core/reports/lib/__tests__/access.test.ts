import { describe, expect, it } from 'vitest'
import { reportAccess, rolesByReport, type ShareRole } from '../access'

type Case = {
    name: string
    owner: string
    published: boolean
    shippedKey: string
    roles: ShareRole[]
    isAdmin: boolean
    want: { canEdit: boolean; canShare: boolean; canPublish: boolean; canDelete: boolean }
}

// The who-sees-what table of the sharing contracts, as the hub and view use it.
const CASES: Case[] = [
    {
        name: 'owner',
        owner: 'me',
        published: false,
        shippedKey: '',
        roles: [],
        isAdmin: false,
        want: { canEdit: true, canShare: true, canPublish: false, canDelete: true },
    },
    {
        name: 'owner who is an admin',
        owner: 'me',
        published: true,
        shippedKey: '',
        roles: [],
        isAdmin: true,
        want: { canEdit: true, canShare: true, canPublish: true, canDelete: true },
    },
    {
        name: 'editor',
        owner: 'ann',
        published: false,
        shippedKey: '',
        roles: ['editor'],
        isAdmin: false,
        want: { canEdit: true, canShare: true, canPublish: false, canDelete: false },
    },
    {
        name: 'editor through a group and a viewer share',
        owner: 'ann',
        published: false,
        shippedKey: '',
        roles: ['viewer', 'editor'],
        isAdmin: false,
        want: { canEdit: true, canShare: true, canPublish: false, canDelete: false },
    },
    {
        name: 'viewer',
        owner: 'ann',
        published: false,
        shippedKey: '',
        roles: ['viewer'],
        isAdmin: false,
        want: { canEdit: false, canShare: false, canPublish: false, canDelete: false },
    },
    {
        name: 'admin with no share on a published report',
        owner: 'ann',
        published: true,
        shippedKey: '',
        roles: [],
        isAdmin: true,
        want: { canEdit: false, canShare: false, canPublish: false, canDelete: true },
    },
    {
        name: 'admin editor of a published report',
        owner: 'ann',
        published: true,
        shippedKey: '',
        roles: ['editor'],
        isAdmin: true,
        want: { canEdit: true, canShare: true, canPublish: true, canDelete: true },
    },
    {
        name: 'member on a published report',
        owner: 'ann',
        published: true,
        shippedKey: '',
        roles: [],
        isAdmin: false,
        want: { canEdit: false, canShare: false, canPublish: false, canDelete: false },
    },
    {
        name: 'owner of a shipped report',
        owner: 'me',
        published: true,
        shippedKey: 'zoo.feed_costs',
        roles: [],
        isAdmin: true,
        want: { canEdit: true, canShare: true, canPublish: true, canDelete: false },
    },
]

describe('reportAccess', () => {
    for (const c of CASES) {
        it(c.name, () => {
            const access = reportAccess({
                owner: c.owner,
                published: c.published,
                shippedKey: c.shippedKey,
                userId: 'me',
                roles: c.roles,
                isAdmin: c.isAdmin,
            })
            expect({
                canEdit: access.canEdit,
                canShare: access.canShare,
                canPublish: access.canPublish,
                canDelete: access.canDelete,
            }).toEqual(c.want)
            expect(access.isOwner).toBe(c.owner === 'me')
            expect(access.isShipped).toBe(c.shippedKey !== '')
        })
    }
})

describe('rolesByReport', () => {
    it('groups a reader’s direct and derived shares by report', () => {
        const roles = rolesByReport([
            { report: 'r1', role: 'viewer' },
            { report: 'r1', role: 'editor' },
            { report: 'r2', role: 'viewer' },
        ])
        expect(roles.get('r1')).toEqual(['viewer', 'editor'])
        expect(roles.get('r2')).toEqual(['viewer'])
        expect(roles.get('r3')).toBeUndefined()
    })
})
