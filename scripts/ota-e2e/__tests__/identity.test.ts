import { describe, expect, it } from 'vitest'
import { classifyBundleId, embeddedIdForVersion, parseCurrentIdFromLogLine } from '../identity'

describe('embeddedIdForVersion', () => {
    it('formats the embedded id from an app version', () => {
        expect(embeddedIdForVersion('1.13.7')).toBe('embedded-1.13.7')
    })
})

describe('classifyBundleId', () => {
    it('recognizes an embedded id', () => {
        expect(classifyBundleId('embedded-1.13.7')).toBe('embedded')
    })
    it('recognizes a server build id', () => {
        expect(classifyBundleId('build-1718200000000-ios')).toBe('server')
    })
    it('returns unknown for anything else', () => {
        expect(classifyBundleId('')).toBe('unknown')
        expect(classifyBundleId('garbage')).toBe('unknown')
    })
})

describe('parseCurrentIdFromLogLine', () => {
    it('extracts q.currentId from a slog key=value line', () => {
        const line =
            'time=2026-06-12T10:00:00Z level=INFO msg="app-update: request" q.platform=ios q.currentId=embedded-1.13.7 server.bundleCount=1'
        expect(parseCurrentIdFromLogLine(line)).toBe('embedded-1.13.7')
    })
    it('extracts a quoted q.currentId value', () => {
        const line = 'msg="app-update: request" q.currentId="build-1718200000000-ios"'
        expect(parseCurrentIdFromLogLine(line)).toBe('build-1718200000000-ios')
    })
    it('returns null for an empty quoted q.currentId', () => {
        expect(parseCurrentIdFromLogLine('msg="app-update: request" q.currentId=""')).toBeNull()
    })
    it('returns null when the line is not an app-update request', () => {
        expect(parseCurrentIdFromLogLine('some other log line')).toBeNull()
    })
    it('returns null when q.currentId is absent', () => {
        expect(parseCurrentIdFromLogLine('msg="app-update: request" q.platform=ios')).toBeNull()
    })
})
