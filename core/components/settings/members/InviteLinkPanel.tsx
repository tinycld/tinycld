import { useQuery } from '@tanstack/react-query'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import * as Clipboard from 'expo-clipboard'
import { useState } from 'react'
import { Pressable, Text, TextInput, View } from 'react-native'

export type InviteLinkPanelProps = {
    userId: string
    initialUrl?: string
    /** The address the invite was just emailed to, if any. */
    emailedTo?: string
}

type LinkState =
    | { kind: 'loading' }
    | { kind: 'ready'; url: string }
    | { kind: 'expired' }
    | { kind: 'error'; message: string }

export function InviteLinkPanel({ userId, initialUrl, emailedTo }: InviteLinkPanelProps) {
    // A URL from rotate() (or the initialUrl prop) short-circuits the fetch —
    // both hand us a ready link with no round-trip needed.
    const [overrideUrl, setOverrideUrl] = useState<string | undefined>(initialUrl)

    const linkQuery = useQuery({
        queryKey: ['invite-link', userId],
        queryFn: () =>
            pb.send<{ inviteUrl: string }>(`/api/invite-link/${userId}`, { method: 'GET' }),
        enabled: !overrideUrl,
        retry: false,
    })

    const rotate = useMutation({
        mutationFn: async () => {
            return pb.send<{ inviteUrl: string }>(`/api/invite-link/${userId}/rotate`, {
                method: 'POST',
            })
        },
        onSuccess: data => setOverrideUrl(data.inviteUrl),
    })

    const state = deriveLinkState(overrideUrl, linkQuery)

    if (state.kind === 'loading') {
        return (
            <View testID="invite-link-panel-loading">
                <Text className="text-muted-foreground">Loading…</Text>
            </View>
        )
    }
    if (state.kind === 'expired') {
        return <ExpiredView onRotate={() => rotate.mutate()} pending={rotate.isPending} />
    }
    if (state.kind === 'error') {
        return (
            <View testID="invite-link-panel-error">
                <Text className="text-destructive">{state.message}</Text>
            </View>
        )
    }
    return (
        <ReadyView
            url={state.url}
            userId={userId}
            emailedTo={emailedTo}
            onRotate={() => rotate.mutate()}
            rotatePending={rotate.isPending}
        />
    )
}

// Maps the invite-link query (or a short-circuit override URL) into the panel's
// view state. A 404 from the endpoint means the link expired; anything else is a
// surfaced error.
function deriveLinkState(
    overrideUrl: string | undefined,
    query: ReturnType<typeof useQuery<{ inviteUrl: string }>>
): LinkState {
    if (overrideUrl) return { kind: 'ready', url: overrideUrl }
    if (query.isPending) return { kind: 'loading' }
    if (query.isError) {
        const status = (query.error as { status?: number })?.status
        if (status === 404) return { kind: 'expired' }
        const message =
            (query.error as { message?: string })?.message ?? 'Failed to load invite link'
        return { kind: 'error', message }
    }
    return { kind: 'ready', url: query.data.inviteUrl }
}

function ExpiredView({ onRotate, pending }: { onRotate: () => void; pending: boolean }) {
    return (
        <View testID="invite-link-panel-expired" className="gap-2">
            <Text className="text-foreground">This invite has expired.</Text>
            <ServerActionButton
                variant="link"
                testID="invite-link-rotate"
                onPress={onRotate}
                isDisabled={pending}
            >
                <ButtonText>{pending ? 'Generating…' : 'Generate new link'}</ButtonText>
            </ServerActionButton>
        </View>
    )
}

type ReadyViewProps = {
    url: string
    userId: string
    emailedTo?: string
    onRotate: () => void
    rotatePending: boolean
}

function ReadyView({ url, userId, emailedTo, onRotate, rotatePending }: ReadyViewProps) {
    const [copied, setCopied] = useState(false)
    const [altEmail, setAltEmail] = useState('')
    const [showSend, setShowSend] = useState(false)

    const send = useMutation({
        mutationFn: async () => {
            return pb.send<{ delivered: true }>(`/api/invite-link/${userId}/send`, {
                method: 'POST',
                body: JSON.stringify({ email: altEmail }),
                headers: { 'Content-Type': 'application/json' },
            })
        },
    })

    const copy = async () => {
        await Clipboard.setStringAsync(url)
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
    }

    return (
        <View testID="invite-link-panel-ready" className="gap-3">
            <EmailedNotice emailedTo={emailedTo} />
            <Text testID="invite-link-url" className="text-foreground" selectable>
                {url}
            </Text>
            <Pressable testID="invite-link-copy" onPress={copy}>
                <Text className="text-primary">{copied ? 'Copied!' : 'Copy link'}</Text>
            </Pressable>
            <ServerActionButton
                variant="link"
                testID="invite-link-rotate"
                onPress={onRotate}
                isDisabled={rotatePending}
            >
                <ButtonText className="text-muted-foreground">
                    {rotatePending ? 'Generating…' : 'Generate new link'}
                </ButtonText>
            </ServerActionButton>

            <Pressable testID="invite-link-send-toggle" onPress={() => setShowSend(s => !s)}>
                <Text className="text-foreground">{sendToggleLabel(showSend, emailedTo)}</Text>
            </Pressable>
            <SendForm
                isVisible={showSend}
                altEmail={altEmail}
                setAltEmail={setAltEmail}
                send={send}
            />
        </View>
    )
}

function EmailedNotice({ emailedTo }: { emailedTo?: string }) {
    if (!emailedTo) return null
    return (
        <Text testID="invite-link-emailed" className="text-foreground">
            We emailed this link to {emailedTo}.
        </Text>
    )
}

function sendToggleLabel(showSend: boolean, emailedTo: string | undefined) {
    if (showSend) return 'Hide email send'
    return emailedTo ? 'Email this link to another address' : 'Email this link'
}

type SendFormProps = {
    isVisible: boolean
    altEmail: string
    setAltEmail: (v: string) => void
    send: ReturnType<typeof useMutation<{ delivered: true }, Error, void>>
}

function SendForm({ isVisible, altEmail, setAltEmail, send }: SendFormProps) {
    if (!isVisible) return null
    return (
        <View className="gap-2">
            <TextInput
                testID="invite-link-alt-email"
                value={altEmail}
                onChangeText={setAltEmail}
                placeholder="recipient@example.com"
                autoCapitalize="none"
                keyboardType="email-address"
                className="text-foreground border-border border rounded p-2"
            />
            <ServerActionButton
                variant="link"
                testID="invite-link-send"
                onPress={() => send.mutate()}
                isDisabled={send.isPending || !altEmail}
            >
                <ButtonText>
                    {send.isPending ? 'Sending…' : send.isSuccess ? 'Sent' : 'Send'}
                </ButtonText>
            </ServerActionButton>
            {send.isError && (
                <Text testID="invite-link-send-error" className="text-destructive">
                    {extractSendError(send.error)}
                </Text>
            )}
        </View>
    )
}

function extractSendError(err: unknown): string {
    if (err && typeof err === 'object') {
        const e = err as { response?: { error?: string }; message?: string }
        if (e.response?.error) return e.response.error
        if (e.message) return e.message
    }
    return 'Send failed'
}
