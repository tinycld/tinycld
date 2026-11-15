import type { ComponentType } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { ReportPanelProps } from '../../hooks/usePanelTitle'
import type { ReportPanelEntry } from '../record-panel'
import { reloadableChunk } from '../registered-panels'

const Panel: ComponentType<ReportPanelProps> = () => null

describe('reloadableChunk (R14)', () => {
    it('keeps a failed chunk until reload, then loads it again', async () => {
        const load = vi
            .fn<ReportPanelEntry['load']>()
            .mockRejectedValueOnce(new Error('offline'))
            .mockResolvedValue({ default: Panel })
        const chunk = reloadableChunk(load)
        await expect(chunk.Component.preload()).rejects.toThrow('offline')
        // lazyChunk keeps its first promise: with no reload the panel stays failed.
        await expect(chunk.Component.preload()).rejects.toThrow('offline')
        expect(load).toHaveBeenCalledTimes(1)
        chunk.reload()
        await expect(chunk.Component.preload()).resolves.toEqual({ default: Panel })
        expect(load).toHaveBeenCalledTimes(2)
    })
})
