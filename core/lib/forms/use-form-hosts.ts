import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import { CORE_FORM_HOST_ENTRY, deriveFormHosts, type FormHost } from '../packages/derive-form-hosts'

export type { FormHost }

/** Core's hosts and the installed packages' hosts. Static for the life of the app, like the package registry. */
export const formHosts: FormHost[] = deriveFormHosts([CORE_FORM_HOST_ENTRY, ...tinycldConfig])

export function useFormHosts(): FormHost[] {
    return formHosts
}

export function useFormHost(key: string | undefined): FormHost | undefined {
    return formHosts.find(h => h.key === key)
}
