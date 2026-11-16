import { z } from 'zod'
import type { ExportFormat } from './export-client'

// The "Email report" form: its addresses, its rules and its defaults. The server
// (core/server/reports/export_email.go) checks the same bounds.

const MAX_ADDRESSES = 10
const MAX_SUBJECT = 200
const MAX_MESSAGE = 5000

const ADDRESS_SPLIT = /[\s,;]+/

/** The addresses of a To or Cc field: split on commas, semicolons and white space, in order. */
export function parseAddresses(text: string): string[] {
    return text.split(ADDRESS_SPLIT).filter(part => part !== '')
}

const emailAddress = z.email()

function badAddressOf(addresses: readonly string[]): string | undefined {
    return addresses.find(address => !emailAddress.safeParse(address).success)
}

const FORMATS = ['pdf', 'xlsx', 'csv'] as const satisfies readonly ExportFormat[]

export const reportEmailSchema = z
    .object({
        to: z.string(),
        cc: z.string(),
        subject: z
            .string()
            .trim()
            .min(1, 'Enter a subject')
            .max(MAX_SUBJECT, `Use at most ${MAX_SUBJECT} characters`),
        message: z.string().max(MAX_MESSAGE, 'Use at most 5,000 characters'),
        format: z.enum(FORMATS),
    })
    .superRefine((values, ctx) => {
        const to = parseAddresses(values.to)
        const cc = parseAddresses(values.cc)
        const badTo = badAddressOf(to)
        const badCc = badAddressOf(cc)
        if (badTo !== undefined) {
            ctx.addIssue({
                code: 'custom',
                path: ['to'],
                message: `${badTo} is not an email address`,
            })
        } else if (to.length === 0) {
            ctx.addIssue({ code: 'custom', path: ['to'], message: 'Enter at least one address' })
        } else if (to.length + cc.length > MAX_ADDRESSES) {
            ctx.addIssue({
                code: 'custom',
                path: ['to'],
                message: `Use at most ${MAX_ADDRESSES} addresses in To and Cc`,
            })
        }
        if (badCc !== undefined) {
            ctx.addIssue({
                code: 'custom',
                path: ['cc'],
                message: `${badCc} is not an email address`,
            })
        }
    })

export type ReportEmailValues = z.input<typeof reportEmailSchema>

/**
 * The form's starting values: no address, the subject "<title> — <period>", a
 * message that names the report, its heading and its period, signed with the
 * sender's name, and PDF. A report with no period leaves the period out.
 */
export function emailDefaults(input: {
    title: string
    heading: string
    periodWords: string
    senderName: string
}): ReportEmailValues {
    const hasPeriod = input.periodWords !== ''
    const subject = hasPeriod ? `${input.title} — ${input.periodWords}` : input.title
    const of = hasPeriod ? `${input.heading}, ${input.periodWords}` : input.heading
    return {
        to: '',
        cc: '',
        subject,
        message: `Hello,\n\nAttached is the ${input.title} report of ${of}.\n\n${input.senderName}`,
        format: 'pdf',
    }
}

/** The sender's name as the message signs it: the name, else the email's local part (Go senderName). */
export function senderNameOf(user: { name: string; email: string }): string {
    const name = user.name.trim()
    if (name !== '') return name
    return user.email.split('@')[0] ?? ''
}

/** "Report sent to <first>", with "and 1 other" or "and <n> others" over To and Cc. */
export function sentToastTitle(to: readonly string[], cc: readonly string[]): string {
    const all = [...to, ...cc]
    const head = `Report sent to ${all[0] ?? ''}`
    const others = all.length - 1
    if (others <= 0) return head
    if (others === 1) return `${head} and 1 other`
    return `${head} and ${others} others`
}
