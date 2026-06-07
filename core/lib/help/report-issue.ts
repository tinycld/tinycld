import { Linking } from 'react-native'

export interface ReportIssueArgs {
    repoUrl: string
    issueTemplate?: string
    pkgName: string
    pkgSlug: string
    pkgVersion: string
    appVersion: string
    commit: string
    platform: string
}

export function buildIssueUrl(args: ReportIssueArgs): string {
    const base = args.repoUrl.replace(/\/+$/, '')
    const title = `[${args.pkgSlug}] `
    const body = buildIssueBody(args)
    const params = new URLSearchParams({ title, body })
    if (args.issueTemplate) params.set('template', args.issueTemplate)
    return `${base}/issues/new?${params.toString()}`
}

export function buildIssueBody(args: ReportIssueArgs): string {
    return [
        '<!-- Describe the issue below. Diagnostic info was filled in automatically. -->',
        '',
        `- **Package:** ${args.pkgName} (\`@tinycld/${args.pkgSlug}\` v${args.pkgVersion})`,
        `- **App version:** ${args.appVersion} (${args.commit})`,
        `- **Platform:** ${args.platform}`,
        '',
        '---',
        '',
        '',
    ].join('\n')
}

export function openPackageIssue(args: ReportIssueArgs): void {
    Linking.openURL(buildIssueUrl(args)).catch(() => {})
}
