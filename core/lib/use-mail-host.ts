import { serverHostname } from './server-host'
import { useOrgInfo } from './use-org-info'

/**
 * The host mail apps connect to for IMAP and SMTP. A deployment can name a
 * host of its own; when it names none, this is the web host the app talks
 * to. Null while the answer is not known yet.
 */
export function useMailHost(): string | null {
    const { mailHost, isPending } = useOrgInfo()
    if (isPending) return null
    return mailHost || serverHostname()
}
