import { describe, expect, it } from 'vitest'
import { buildUniwindSources } from '../gen-uniwind'

describe('buildUniwindSources', () => {
    it('emits one @source per package real path', () => {
        const css = buildUniwindSources([
            { packageName: '@tinycld/doodads', packageDir: '/abs/doodads' },
            { packageName: '@tinycld/core', packageDir: '/abs/core' },
        ])
        expect(css).toContain('@source "/abs/doodads";  /* @tinycld/doodads */')
        expect(css).toContain('@source "/abs/core";  /* @tinycld/core */')
    })
})
