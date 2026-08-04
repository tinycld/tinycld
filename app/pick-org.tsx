import { PreAuthScreen } from '@tinycld/core/components/connect/PreAuthScreen'
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { hostnameOf, isOrgUnderApex, orgUrlUnderApex, slugUnderApex } from '@tinycld/core/lib/apex'
import { getCoreConfigOptional } from '@tinycld/core/lib/core-config'
import { ReloadUnavailableError } from '@tinycld/core/lib/reload-js-context'
import { normalizeAddress, probeServer, setResolvedAddress } from '@tinycld/core/lib/server-address'
import { readServers, setActiveServer } from '@tinycld/core/lib/servers'
import { switchToServer } from '@tinycld/core/lib/switch-server'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { router, useLocalSearchParams } from 'expo-router'
import { Building2, ChevronRight, Server } from 'lucide-react-native'
import { useEffect, useState } from 'react'
import { Pressable, Text, View } from 'react-native'

// The native counterpart to the router's apex org-finder page (multi-org,
// internal/webpage). Reaching the apex means the user is at an address that
// HOSTS organizations rather than being one — there is no PocketBase behind it,
// so a sign-in panel here could never succeed. This screen asks the one question
// that actually unblocks them: which org?
//
// Deliberately mirrors the web page's two affordances and no more: pick an org
// this device already knows, or type your org's name. There is no "create an
// organization" — provisioning is superuser-only on the admin subdomain, so
// offering it would promise a self-serve signup that does not exist.

const slugSchema = z.object({
    slug: z.string().min(1, "Enter your organization's name."),
})

interface KnownOrg {
    origin: string
    slug: string
}

// The orgs on THIS router that this device has connected to before. Saved
// servers already model an org as an origin (lib/servers.ts says so explicitly),
// so there is no separate org list to maintain — just the subset living under
// this apex. A self-hosted box in the list is correctly excluded: it is not an
// org of this router, and its row would switch the user somewhere unrelated.
function useKnownOrgs(apexHostname: string | null): KnownOrg[] {
    const [orgs, setOrgs] = useState<KnownOrg[]>([])

    useEffect(() => {
        if (!apexHostname) return
        let cancelled = false
        readServers().then(servers => {
            if (cancelled) return
            const known = servers
                .filter(s => isOrgUnderApex(s.origin, apexHostname))
                .map(s => ({ origin: s.origin, slug: slugUnderApex(s.origin, apexHostname) ?? '' }))
            setOrgs(known)
        })
        return () => {
            cancelled = true
        }
    }, [apexHostname])

    return orgs
}

export default function PickOrg() {
    const { apex } = useLocalSearchParams<{ apex?: string }>()
    const config = getCoreConfigOptional()
    const brandName = config?.brandName ?? 'TinyCld'

    const apexOrigin = apex ? normalizeAddress(apex) : (config?.defaultServer ?? '')
    const apexHostname = hostnameOf(apexOrigin)
    const knownOrgs = useKnownOrgs(apexHostname)

    const [busy, setBusy] = useState(false)
    const [submitError, setSubmitError] = useState<string | null>(null)

    const muted = useThemeColor('muted-foreground')
    const fg = useThemeColor('foreground')

    const { control, handleSubmit } = useForm({
        resolver: zodResolver(slugSchema),
        defaultValues: { slug: '' },
        mode: 'onChange',
    })

    // Connecting to an org is the ordinary first-run path, not a switch: there is
    // no active server to preserve, because the apex was never admitted as one.
    async function connectToOrg(origin: string) {
        await probeServer(origin)
        await setActiveServer(origin)
        setResolvedAddress(origin)
        router.replace('/')
    }

    // A row for an org already in the saved list, on the other hand, may well be
    // a switch away from another org — switchToServer handles both and restarts
    // the JS context so the target org's own bundle loads.
    async function onPickKnown(origin: string) {
        setSubmitError(null)
        setBusy(true)
        try {
            await switchToServer(origin)
        } catch (err) {
            setSubmitError(describeFailure(err, origin))
            setBusy(false)
        }
    }

    const onSubmitSlug = handleSubmit(async ({ slug }) => {
        setSubmitError(null)
        if (!apexHostname) {
            setSubmitError('This address does not look like a TinyCld host.')
            return
        }
        const origin = orgUrlUnderApex(slug.trim().toLowerCase(), apexHostname)
        if (!origin) {
            setSubmitError('Organization names are lowercase letters, digits, and hyphens.')
            return
        }
        setBusy(true)
        try {
            await connectToOrg(origin)
        } catch (err) {
            setSubmitError(describeFailure(err, origin))
            setBusy(false)
        }
    })

    return (
        <PreAuthScreen>
            <DocumentTitle title="Choose an organization" includeOrg={false} />
            <View className="flex-row items-center gap-2 mb-2">
                <View className="w-[18px] h-px bg-primary" />
                <Text
                    className="text-[11px] font-semibold text-primary"
                    style={{ letterSpacing: 2 }}
                >
                    CHOOSE AN ORGANIZATION
                </Text>
            </View>

            <Text
                className="text-foreground text-[32px] font-semibold"
                style={{ lineHeight: 36, letterSpacing: -0.8, fontFamily: 'Georgia' }}
            >
                Find your{' '}
                <Text className="italic font-normal text-primary" style={{ fontFamily: 'Georgia' }}>
                    organization.
                </Text>
            </Text>

            <Text
                className="text-foreground text-[15px] mt-3.5"
                style={{ lineHeight: 22, opacity: 0.78, maxWidth: 360 }}
            >
                {apexHostname ?? brandName} hosts many organizations, each with its own address.
                Pick one you've used on this device, or enter your organization's name.
            </Text>

            <KnownOrgsSection
                orgs={knownOrgs}
                apexHostname={apexHostname}
                disabled={busy}
                onPick={onPickKnown}
            />

            <View className="mt-7">
                <TextInput
                    control={control}
                    name="slug"
                    label="Your organization's name"
                    placeholder="your-org"
                    autoCapitalize="none"
                    autoCorrect={false}
                    autoComplete="off"
                    hint={apexHostname ? `We'll open your-org.${apexHostname}` : undefined}
                />
            </View>

            {submitError ? (
                <View className="mt-3 rounded-lg p-3 bg-danger-soft">
                    <Text className="text-xs text-danger">{submitError}</Text>
                </View>
            ) : null}

            <Pressable
                onPress={onSubmitSlug}
                disabled={busy}
                className={`mt-4 bg-foreground rounded-2xl py-4 px-5 items-center justify-center relative overflow-hidden ${busy ? 'opacity-[0.55]' : 'opacity-100'}`}
            >
                <View className="absolute top-0 bottom-0 left-0 w-1 bg-primary" />
                <Text className="text-[15px] font-semibold text-background">
                    {busy ? 'Connecting…' : 'Continue'}
                </Text>
            </Pressable>

            <Text className="text-muted-foreground text-[13px] mt-5" style={{ lineHeight: 19 }}>
                Not sure of the address? It's in your invitation email, or ask your organization's
                administrator.
            </Text>

            <View className="flex-1" />

            {/* A self-hoster who tapped the hosted default must not be stranded
                    here — /connect is the surface that takes an arbitrary address. */}
            <Pressable
                onPress={() => router.replace('/connect')}
                disabled={busy}
                className={`mt-8 rounded-xl border border-border bg-surface flex-row items-center gap-3 px-4 py-3.5 ${busy ? 'opacity-50' : 'opacity-100'}`}
            >
                <Server size={16} color={fg} />
                <Text className="text-foreground text-sm font-medium flex-1">
                    Use a different server
                </Text>
                <ChevronRight size={16} color={muted} />
            </Pressable>
        </PreAuthScreen>
    )
}

function KnownOrgsSection({
    orgs,
    apexHostname,
    disabled,
    onPick,
}: {
    orgs: KnownOrg[]
    apexHostname: string | null
    disabled: boolean
    onPick: (origin: string) => void
}) {
    const muted = useThemeColor('muted-foreground')
    const fg = useThemeColor('foreground')
    if (orgs.length === 0) return null
    return (
        <View className="mt-7">
            <Text
                className="text-[11px] font-semibold text-muted-foreground mb-2"
                style={{ letterSpacing: 1.6 }}
            >
                YOUR ORGANIZATIONS
            </Text>
            {orgs.map(org => (
                <Pressable
                    key={org.origin}
                    onPress={() => onPick(org.origin)}
                    disabled={disabled}
                    className={`rounded-xl border border-border bg-surface flex-row items-center gap-3 px-4 py-3.5 mb-2 ${disabled ? 'opacity-50' : 'opacity-100'}`}
                >
                    <Building2 size={16} color={fg} />
                    <View className="flex-1">
                        <Text className="text-foreground text-sm font-medium">{org.slug}</Text>
                        <Text className="text-muted-foreground text-xs mt-px">
                            {org.slug}.{apexHostname}
                        </Text>
                    </View>
                    <ChevronRight size={16} color={muted} />
                </Pressable>
            ))}
        </View>
    )
}

// A refused switch is not a connection failure: the org was reachable and saved,
// only the JS-context restart is unavailable (dev builds have no reload
// mechanism). Saying "couldn't reach" would send the user debugging a network
// problem that does not exist. Mirrors connect.tsx's describeFailure.
function describeFailure(err: unknown, label: string): string {
    if (err instanceof ReloadUnavailableError) {
        return `Saved ${label}. Restart the app to finish opening it.`
    }
    const reason = err instanceof Error ? err.message : 'Connection failed'
    return `Couldn't reach ${label}: ${reason}`
}
