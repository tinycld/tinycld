import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { ALL_OPS } from '../helpers'

describe('CLI destination help', () => {
    it('names every condition operator the rules engine accepts', () => {
        const help = readFileSync(join(__dirname, '../../../../cli/form_help/destination.txt'), 'utf8')
        for (const op of ALL_OPS) expect(help).toContain(op)
    })
})
