import { describe, expect, it, vi } from 'vitest'
import { type FocusTarget, figureKey, focusFigure, registerFigure } from '../figure-focus'

const node = () =>
    ({ focus: vi.fn() }) as unknown as FocusTarget & { focus: ReturnType<typeof vi.fn> }

describe('focusFigure', () => {
    it('focuses the figure drawn under the key', () => {
        const figure = node()
        const forget = registerFigure(figureKey('main', 'amount', 'g:1'), figure)
        focusFigure(figureKey('main', 'amount', 'g:1'))
        expect(figure.focus).toHaveBeenCalledOnce()
        forget()
    })

    it('does nothing for a figure no longer drawn', () => {
        const figure = node()
        registerFigure(figureKey('main', 'amount', 'g:2'), figure)()
        focusFigure(figureKey('main', 'amount', 'g:2'))
        expect(figure.focus).not.toHaveBeenCalled()
    })

    it('keeps a newer figure of the key when an older one leaves', () => {
        const older = node()
        const newer = node()
        const forgetOlder = registerFigure(figureKey('main', 'amount', 'g:3'), older)
        registerFigure(figureKey('main', 'amount', 'g:3'), newer)
        forgetOlder()
        focusFigure(figureKey('main', 'amount', 'g:3'))
        expect(newer.focus).toHaveBeenCalledOnce()
    })

    it('files the same column and row of two views apart', () => {
        const main = node()
        const drill = node()
        registerFigure(figureKey('main', 'amount', 'g:4'), main)
        const forgetDrill = registerFigure(figureKey('drill', 'amount', 'g:4'), drill)
        focusFigure(figureKey('drill', 'amount', 'g:4'))
        expect(drill.focus).toHaveBeenCalledOnce()
        expect(main.focus).not.toHaveBeenCalled()
        forgetDrill()
        focusFigure(figureKey('main', 'amount', 'g:4'))
        expect(main.focus).toHaveBeenCalledOnce()
    })
})
