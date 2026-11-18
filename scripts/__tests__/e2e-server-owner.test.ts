import { describe, expect, it } from 'vitest'
import { dataDirRefusal, parseDataDirs, parseListenerPids } from '../e2e-server-owner'

describe('parseListenerPids', () => {
    it('reads the pid lines of lsof -F output', () => {
        expect(parseListenerPids('p4242\np4243\n')).toEqual([4242, 4243])
    })

    it('returns no pid for empty output', () => {
        expect(parseListenerPids('')).toEqual([])
    })
})

describe('parseDataDirs', () => {
    it('takes the dir of each open data.db and ignores its -wal and -shm files', () => {
        const out = [
            'p4242',
            'fcwd',
            'n/ws-a/tinycld',
            'f12',
            'n/ws-a/tinycld/server/pb_test_data/data.db',
            'f13',
            'n/ws-a/tinycld/server/pb_test_data/data.db-wal',
            'f14',
            'n/ws-a/tinycld/server/pb_test_data/auxiliary.db',
            'p4243',
            'f9',
            'n/ws-a/tinycld/server/pb_test_data/data.db',
        ].join('\n')
        expect(parseDataDirs(out)).toEqual(['/ws-a/tinycld/server/pb_test_data'])
    })

    it('keeps a dir with spaces in its name whole', () => {
        expect(parseDataDirs('p1\nf3\nn/my ws/server/pb_test_data/data.db\n')).toEqual([
            '/my ws/server/pb_test_data',
        ])
    })
})

describe('dataDirRefusal', () => {
    const expectedDir = '/ws-a/tinycld/server/pb_test_data'

    it('accepts a server that serves this workspace data dir', () => {
        expect(dataDirRefusal({ port: 7200, expectedDir, serverDirs: [expectedDir] })).toBeNull()
    })

    it('refuses a server from another workspace and names both dirs', () => {
        const otherDir = '/ws-b/tinycld/server/pb_test_data'
        const message = dataDirRefusal({ port: 7200, expectedDir, serverDirs: [otherDir] })
        expect(message).toContain(':7200')
        expect(message).toContain(otherDir)
        expect(message).toContain(expectedDir)
    })

    it('refuses a server whose data dir cannot be read', () => {
        const message = dataDirRefusal({ port: 7201, expectedDir, serverDirs: [] })
        expect(message).toContain(':7201')
        expect(message).toContain(expectedDir)
    })
})
