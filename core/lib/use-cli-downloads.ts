import { useQuery } from '@tanstack/react-query'
import { Platform } from 'react-native'
import { PB_SERVER_ADDR } from './config'

/** One cross-compiled CLI binary the server offers, as listed by the public
 *  GET /api/cli/downloads endpoint (Go: coreserver/cli_downloads.go). */
export interface CliDownload {
    platform: string
    os: string
    arch: string
    filename: string
    size: number
    url: string
}

// Exported for unit testing; prefer useCliDownloads in components.
export async function fetchCliDownloads(): Promise<CliDownload[]> {
    const res = await fetch(`${PB_SERVER_ADDR}/api/cli/downloads`, { cache: 'no-store' })
    if (!res.ok) return []
    const body = (await res.json()) as { downloads?: CliDownload[] }
    return body.downloads ?? []
}

/** The viewer's OS as a CLI target OS, from the web user agent. Detection is
 *  OS-only on purpose: Apple Silicon browsers report an Intel UA, so guessing
 *  macOS arch would confidently offer the wrong binary — the UI lists both
 *  mac entries instead. Returns null on native (a phone isn't a CLI host) and
 *  when the UA is unrecognized. */
export function detectCliOS(userAgent?: string): 'darwin' | 'windows' | 'linux' | null {
    if (Platform.OS !== 'web') return null
    const ua = userAgent ?? (typeof navigator === 'undefined' ? '' : navigator.userAgent)
    if (/Mac OS X|Macintosh/i.test(ua)) return 'darwin'
    if (/Windows/i.test(ua)) return 'windows'
    if (/Linux|X11/i.test(ua)) return 'linux'
    return null
}

/** Stable-sorts the detected OS's entries first so the viewer's own platform
 *  leads the list; within (and without) a match the server order is kept. */
export function orderForViewer(downloads: CliDownload[], os: string | null): CliDownload[] {
    if (!os) return downloads
    return [...downloads.filter(d => d.os === os), ...downloads.filter(d => d.os !== os)]
}

const OS_LABELS: Record<string, string> = {
    darwin: 'macOS',
    linux: 'Linux',
    windows: 'Windows',
}

const ARCH_LABELS: Record<string, string> = {
    arm64: 'arm64',
    amd64: 'x64',
}

export function downloadLabel(d: CliDownload): string {
    if (d.os === 'darwin') {
        return d.arch === 'arm64' ? 'macOS (Apple Silicon)' : 'macOS (Intel)'
    }
    const os = OS_LABELS[d.os] ?? d.os
    const arch = ARCH_LABELS[d.arch] ?? d.arch
    return `${os} (${arch})`
}

export function downloadUrl(d: CliDownload): string {
    return `${PB_SERVER_ADDR}${d.url}`
}

// The set of built binaries is fixed for the lifetime of a running build — it
// only changes across installs/rebuilds. Same caching rationale as
// useReleaseManifest: no refetch churn, no retry hammering; the About panel
// hides the section until data arrives.
export function useCliDownloads() {
    const { data } = useQuery<CliDownload[]>({
        queryKey: ['cli-downloads'],
        queryFn: fetchCliDownloads,
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })
    const detectedOS = detectCliOS()
    return { downloads: orderForViewer(data ?? [], detectedOS), detectedOS }
}
