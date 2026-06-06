import Constants from 'expo-constants'
import { Platform } from 'react-native'
import { getCoreConfigOptional } from '../core-config'
import { usePackage } from '../packages/use-packages'
import { openPackageIssue } from './report-issue'

/**
 * Returns a handler that opens a pre-filled GitHub "new issue" page for the
 * given package, or `null` when the package declares no `repository.url`.
 *
 * Mirrors the diagnostic context assembled by `ReportIssueRow` so the menubar
 * Help menu and the help-drawer row open issues with identical bodies. Callers
 * gate the menu item on the return value being non-null.
 */
export function useReportIssue(pkgSlug: string): (() => void) | null {
    const pkg = usePackage(pkgSlug)
    const repoUrl = pkg?.repository?.url
    if (!pkg || !repoUrl) return null

    return () =>
        openPackageIssue({
            repoUrl,
            issueTemplate: pkg.repository?.issueTemplate,
            pkgName: pkg.name,
            pkgSlug: pkg.slug,
            pkgVersion: pkg.version,
            appVersion: Constants.expoConfig?.version ?? 'unknown',
            commit: (getCoreConfigOptional()?.release ?? 'dev').slice(0, 7),
            platform: Platform.OS,
        })
}
