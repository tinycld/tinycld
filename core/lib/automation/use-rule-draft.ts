// Thin state container for the rule builder: one useState<RuleDraft> + a
// patch callback + submit-time validation. Keystroke-level validation would
// flash "Name is required" while the user is still typing the first
// character — errors only populate after a failed validate() call, mirroring
// the FormErrorSummary idiom this builder hand-rolls (it isn't an RHF form,
// so there's no formState.isSubmitted to key off).
import { useCallback, useState } from 'react'
import type { CatalogResponse } from './api'
import type { RuleDraft } from './draft'
import { validateDraft } from './draft'

export interface UseRuleDraftResult {
    draft: RuleDraft
    patch: (p: Partial<RuleDraft>) => void
    errors: string[] | null
    validate: (catalog: CatalogResponse | undefined) => boolean
}

export function useRuleDraft(initial: RuleDraft): UseRuleDraftResult {
    const [draft, setDraft] = useState<RuleDraft>(initial)
    const [errors, setErrors] = useState<string[] | null>(null)

    const patch = useCallback((p: Partial<RuleDraft>) => {
        setDraft(prev => ({ ...prev, ...p }))
    }, [])

    const validate = useCallback(
        (catalog: CatalogResponse | undefined) => {
            const found = validateDraft(draft, catalog)
            setErrors(found.length > 0 ? found : null)
            return found.length === 0
        },
        [draft]
    )

    return { draft, patch, errors, validate }
}
