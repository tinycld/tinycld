import { ConnectIllustration } from '@tinycld/core/components/connect/ConnectIllustration'
import { PreAuthScreen } from '@tinycld/core/components/connect/PreAuthScreen'
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { ApexServerError } from '@tinycld/core/lib/apex'
import { getCoreConfigOptional } from '@tinycld/core/lib/core-config'
import { PICK_ORG_HREF } from '@tinycld/core/lib/org-routes'
import { isReloadAvailable, ReloadUnavailableError } from '@tinycld/core/lib/reload-js-context'
import { normalizeAddress, probeServer, setResolvedAddress } from '@tinycld/core/lib/server-address'
import { setActiveServer } from '@tinycld/core/lib/servers'
import { switchToServer } from '@tinycld/core/lib/switch-server'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { router, useLocalSearchParams } from 'expo-router'
import { ChevronDown, Globe, Server, X } from 'lucide-react-native'
import { useState } from 'react'
import { KeyboardAvoidingView, Modal, Platform, Pressable, Text, View } from 'react-native'
import { SafeAreaView } from 'react-native-safe-area-context'

const FALLBACK_DEFAULT_SERVER = 'https://tinycld.org'

const urlSchema = z.object({
    url: z.string().min(1, 'Enter a server address.'),
})

// Not every unhappy outcome is a failure. A refused JS-context restart leaves
// the server saved and active, so it reads as 'info' — only a genuinely failed
// connection is 'error'.
interface Notice {
    tone: 'error' | 'info'
    text: string
}

// The screen serves two jobs — first-run "pick a server" and, from Settings,
// "add another one alongside the one you're signed into". Same probe, same
// normalization, different framing.
function screenCopy(isAddMode: boolean, brandName: string) {
    if (isAddMode) {
        return {
            eyebrow: 'ADD A SERVER',
            headline: 'Connect another',
            headlineAccent: 'server.',
            body: `You'll stay signed in to your current server. ${brandName} keeps each one separate — switch between them any time.`,
            sheetOpener: 'Enter a server address',
            sheetTitle: 'Add a server',
            sheetBody: `Enter the address of the other ${brandName} server. We'll check it and add it to your list.`,
        }
    }
    return {
        eyebrow: 'WELCOME',
        headline: 'Pick a place to keep',
        headlineAccent: 'your stuff.',
        body: `${brandName} stores everything on a server you choose — no shared cloud, no telemetry, nothing in our hands.`,
        sheetOpener: 'I host my own server',
        sheetTitle: 'Connect your server',
        sheetBody: `Enter the address where your ${brandName} server is running. We'll check it and remember it for next time.`,
    }
}

export default function Connect() {
    const { backTo, mode } = useLocalSearchParams<{ backTo?: string; mode?: string }>()
    // `?mode=add` reaches here from Settings → Servers → Add server. The
    // difference from the first-run flow is that the CURRENT server's session
    // must survive: we switch to the new server rather than replacing the only
    // one. Tokens are namespaced per server, so simply not clearing is enough.
    const isAddMode = mode === 'add'
    const config = getCoreConfigOptional()
    const brandName = config?.brandName ?? 'TinyCld'
    const defaultServer = config?.defaultServer ?? FALLBACK_DEFAULT_SERVER
    const defaultServerLabel = hostLabel(defaultServer) ?? 'tinycld.org'
    const copy = screenCopy(isAddMode, brandName)

    const [sheetOpen, setSheetOpen] = useState(false)
    const [busyDefault, setBusyDefault] = useState(false)
    const [busyCustom, setBusyCustom] = useState(false)
    const [notice, setNotice] = useState<Notice | null>(null)

    // Adding a server switches to it, and a switch needs a JS-context restart
    // this build may not have (dev bundler, or a binary without reload support).
    // Say so BEFORE the tap — the same thing ServersDrawerSection and UserMenu
    // already do — rather than letting the user discover it from what looks
    // like an error afterwards. Only add-mode switches; first-run does not.
    const warnsAboutRestart = isAddMode && !isReloadAvailable()

    const fg = useThemeColor('foreground')
    const muted = useThemeColor('muted-foreground')
    const backdrop = useThemeColor('overlay-backdrop')

    const { control, handleSubmit, reset } = useForm({
        resolver: zodResolver(urlSchema),
        defaultValues: { url: '' },
        mode: 'onChange',
    })

    async function connectTo(addr: string) {
        // probeServer, not probe: a hosting apex is alive and answers 200, so a
        // liveness check admits it and the app then renders a sign-in panel
        // against a host with no PocketBase. ApexServerError means "this address
        // hosts orgs" — the recovery is to ask which one, not to report an error.
        try {
            await probeServer(addr)
        } catch (err) {
            if (err instanceof ApexServerError) {
                router.replace({
                    pathname: PICK_ORG_HREF,
                    params: { apex: err.apexOrigin },
                })
                return
            }
            throw err
        }

        if (isAddMode) {
            // Adding a second server: persist it, then restart the JS context so
            // the new server's bundle and a clean module graph load. Deliberately
            // does NOT disconnect — the previous server stays signed in.
            // switchToServer throws rather than half-applying when the context
            // cannot be restarted, so the catch below surfaces that.
            await switchToServer(addr)
            return
        }

        // First-run / change-server: setActiveServer is the only sanctioned
        // writer of the active pointer — a raw writeCached would set an active
        // server with no list entry, so it would never appear in the switcher.
        await setActiveServer(addr)
        setResolvedAddress(addr)
        const target = backTo?.startsWith('/') ? backTo : '/'
        router.replace(target)
    }

    // A refused switch is NOT a connection failure — the server was reachable and
    // has been saved; only the JS-context restart is unavailable (dev builds have
    // no reload mechanism). Saying "couldn't reach" there would send the user
    // debugging a network problem that doesn't exist.
    //
    // The tone travels WITH the message rather than being inferred where it is
    // rendered: this outcome is a success with a follow-up step, and showing it
    // in the danger style told the user something had broken when nothing had.
    function describeFailure(err: unknown, label: string): Notice {
        if (err instanceof ReloadUnavailableError) {
            return {
                tone: 'info',
                text: `Saved ${label}. Restart the app to finish switching to it.`,
            }
        }
        const reason = err instanceof Error ? err.message : 'Connection failed'
        return { tone: 'error', text: `Couldn't reach ${label}: ${reason}` }
    }

    async function onUseDefault() {
        setNotice(null)
        setBusyDefault(true)
        try {
            await connectTo(normalizeAddress(defaultServer))
        } catch (err) {
            setNotice(describeFailure(err, defaultServerLabel))
            setBusyDefault(false)
        }
    }

    function openSheet() {
        setNotice(null)
        setSheetOpen(true)
    }

    function closeSheet() {
        setSheetOpen(false)
        setNotice(null)
        reset({ url: '' })
    }

    const onSubmitCustom = handleSubmit(async ({ url }) => {
        setNotice(null)
        setBusyCustom(true)
        const addr = normalizeAddress(url)
        try {
            await connectTo(addr)
        } catch (err) {
            setNotice(describeFailure(err, addr))
            setBusyCustom(false)
        }
    })

    const busy = busyDefault || busyCustom

    return (
        <>
            <PreAuthScreen>
                <DocumentTitle title="Connect" includeOrg={false} />
                <View className="flex-row items-center gap-3">
                    <BrandMark name={brandName} />
                </View>

                <View className="mt-8 mb-6">
                    <ConnectIllustration height={130} />
                </View>

                <View className="flex-row items-center gap-2 mb-2">
                    <View className="w-[18px] h-px bg-primary" />
                    <Text
                        className="text-[11px] font-semibold text-primary"
                        style={{ letterSpacing: 2 }}
                    >
                        {copy.eyebrow}
                    </Text>
                </View>

                <Text
                    className="text-foreground text-[32px] font-semibold"
                    style={{
                        lineHeight: 36,
                        letterSpacing: -0.8,
                        fontFamily: 'Georgia',
                    }}
                >
                    {copy.headline}{' '}
                    <Text
                        className="italic font-normal text-primary"
                        style={{ fontFamily: 'Georgia' }}
                    >
                        {copy.headlineAccent}
                    </Text>
                </Text>

                <Text
                    className="text-foreground text-[15px] mt-3.5"
                    style={{
                        lineHeight: 22,
                        opacity: 0.78,
                        maxWidth: 360,
                    }}
                >
                    {copy.body}
                </Text>

                <View className="mt-5">
                    <RestartNotice isVisible={warnsAboutRestart && !sheetOpen && !notice} />
                    <NoticeBanner notice={notice} isVisible={!sheetOpen} />
                </View>

                <View className="flex-1" />

                <View className="gap-2.5 mt-7">
                    <PrimaryCta
                        testID="connect-use-default"
                        label={busyDefault ? 'Connecting…' : `Use ${defaultServerLabel}`}
                        onPress={onUseDefault}
                        disabled={busy}
                        // In add mode the hosted default is the wrong default: the
                        // user came here to name a specific other server, and is
                        // very likely already signed into the hosted one.
                        isVisible={!isAddMode}
                    />
                    <Pressable
                        onPress={openSheet}
                        disabled={busy}
                        className={`rounded-xl border border-border bg-surface flex-row items-center justify-between px-4 py-3.5 ${busy ? 'opacity-50' : 'opacity-100'}`}
                    >
                        <View className="flex-row items-center gap-3">
                            <Server size={16} color={fg} />
                            <Text className="text-foreground text-sm font-medium">
                                {copy.sheetOpener}
                            </Text>
                        </View>
                        <ChevronDown size={16} color={muted} />
                    </Pressable>
                </View>
            </PreAuthScreen>

            <Modal
                visible={sheetOpen}
                transparent
                animationType="slide"
                onRequestClose={closeSheet}
            >
                <KeyboardAvoidingView
                    behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
                    className="flex-1"
                >
                    <Pressable
                        onPress={closeSheet}
                        className="flex-1"
                        style={{ backgroundColor: backdrop }}
                    />
                    <View
                        className="bg-background px-6 pt-3 pb-8"
                        style={{
                            borderTopLeftRadius: 24,
                            borderTopRightRadius: 24,
                        }}
                    >
                        <SafeAreaView edges={['bottom']}>
                            <View className="w-[38px] h-1 rounded-sm bg-border self-center mb-[18px]" />
                            <View className="flex-row items-start justify-between mb-3">
                                <View className="flex-1 pr-3">
                                    <Text
                                        className="text-foreground text-[22px] font-semibold"
                                        style={{
                                            letterSpacing: -0.4,
                                            fontFamily: 'Georgia',
                                        }}
                                    >
                                        {copy.sheetTitle}
                                    </Text>
                                    <Text
                                        className="mt-1.5 text-[13px] text-muted-foreground"
                                        style={{ lineHeight: 19 }}
                                    >
                                        {copy.sheetBody}
                                    </Text>
                                </View>
                                <Pressable
                                    onPress={closeSheet}
                                    accessibilityLabel="Close"
                                    className="w-8 h-8 rounded-full border border-border items-center justify-center"
                                >
                                    <X size={14} color={fg} />
                                </Pressable>
                            </View>

                            <TextInput
                                control={control}
                                name="url"
                                label="Server address"
                                placeholder="https://pb.example.com"
                                autoCapitalize="none"
                                autoCorrect={false}
                                keyboardType="url"
                                hint="Usually starts with https://. If you skip the protocol, we'll add one."
                                autoFocus
                            />

                            <RestartNotice isVisible={warnsAboutRestart && !notice} />

                            <NoticeBanner notice={notice} className="mb-3" />

                            <PrimaryCta
                                label={busyCustom ? 'Connecting…' : 'Connect'}
                                onPress={onSubmitCustom}
                                disabled={busy}
                            />
                        </SafeAreaView>
                    </View>
                </KeyboardAvoidingView>
            </Modal>
        </>
    )
}

// One place decides how a notice looks, so a success-with-a-caveat can never
// pick up the danger styling again by being rendered at a site that assumes
// every message is an error.
function NoticeBanner({
    notice,
    isVisible = true,
    className = '',
}: {
    notice: Notice | null
    isVisible?: boolean
    className?: string
}) {
    if (!notice || !isVisible) return null
    const isError = notice.tone === 'error'
    return (
        <View
            className={`rounded-lg p-3 ${isError ? 'bg-danger-soft' : 'bg-surface-secondary'} ${className}`}
        >
            <Text className={`text-xs ${isError ? 'text-danger' : 'text-foreground'}`}>
                {notice.text}
            </Text>
        </View>
    )
}

// Told BEFORE the tap, not after: this build cannot restart its own JS context,
// so a switch needs a manual relaunch to finish. Mirrors the notice
// ServersDrawerSection shows for the same condition.
function RestartNotice({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="text-[11px] text-muted-foreground mb-3">
            Switching requires a restart in this build.
        </Text>
    )
}

function BrandMark({ name }: { name: string }) {
    const initial = name.charAt(0).toUpperCase()
    return (
        <View className="flex-row items-center gap-2.5">
            <View className="w-9 h-9 rounded-[10px] bg-foreground items-center justify-center relative">
                <Text
                    className="text-background text-lg font-bold"
                    style={{ fontFamily: 'Georgia' }}
                >
                    {initial}
                </Text>
                <View className="absolute top-[5px] right-[5px] w-1.5 h-1.5 rounded-full bg-primary" />
            </View>
            <View>
                <Text
                    className="text-foreground text-[15px] font-semibold"
                    style={{ fontFamily: 'Georgia' }}
                >
                    {name}
                </Text>
                <Text
                    className="text-[9px] text-muted-foreground mt-px font-semibold"
                    style={{ letterSpacing: 1.6 }}
                >
                    YOUR DATA, AT HOME
                </Text>
            </View>
        </View>
    )
}

function PrimaryCta({
    label,
    onPress,
    disabled,
    testID,
    isVisible = true,
}: {
    label: string
    onPress: () => void
    disabled?: boolean
    testID?: string
    isVisible?: boolean
}) {
    const bg = useThemeColor('background')
    if (!isVisible) return null
    return (
        <Pressable
            testID={testID}
            onPress={onPress}
            disabled={disabled}
            className={`bg-foreground rounded-2xl py-4 px-5 items-center justify-center relative overflow-hidden ${disabled ? 'opacity-[0.55]' : 'opacity-100'}`}
        >
            <View className="absolute top-0 bottom-0 left-0 w-1 bg-primary" />
            <View className="flex-row items-center gap-2">
                <Globe size={16} color={bg} />
                <Text className="text-[15px] font-semibold text-background">{label}</Text>
            </View>
        </Pressable>
    )
}

function hostLabel(url: string | undefined): string | null {
    if (!url) return null
    try {
        return new URL(url).host
    } catch {
        return null
    }
}
