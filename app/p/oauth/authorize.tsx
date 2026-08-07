import { useQuery } from '@tanstack/react-query'
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { ScopeList } from '@tinycld/core/components/oauth/ScopeList'
import { AuthGate } from '@tinycld/core/components/workspace/AuthGate'
import { useAuth } from '@tinycld/core/lib/auth'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useLocalSearchParams } from 'expo-router'
import { useState } from 'react'
import { ActivityIndicator, Text, TextInput, View } from 'react-native'

interface AuthorizeInfo {
    client_name: string
    scopes: string[]
    expires_at: string
}

type Decision = 'approved' | 'denied' | null

// A user_code is short enough to type by hand from a terminal prompt (e.g.
// "WDJB-MJHT"), so the info lookup only fires once enough characters exist —
// this avoids a request-per-keystroke against /oauth/authorize/info while
// still working for a code pasted or pre-filled from the URL in one shot.
const MIN_CODE_LENGTH = 9

function useAuthorizeInfo(code: string, enabled: boolean) {
    return useQuery<AuthorizeInfo>({
        queryKey: ['oauth-authorize-info', code],
        enabled: enabled && code.length >= MIN_CODE_LENGTH,
        queryFn: () =>
            pb.send<AuthorizeInfo>('/oauth/authorize/info', {
                method: 'GET',
                query: { user_code: code },
            }),
        retry: false,
    })
}

function useDeviceDecision(code: string) {
    const [decision, setDecision] = useState<Decision>(null)

    const approve = useMutation({
        mutationFn: async (deviceLabel: string) => {
            const body = new FormData()
            body.append('user_code', code)
            body.append('device_label', deviceLabel)
            await pb.send('/oauth/authorize/approve', { method: 'POST', body })
        },
        onSuccess: () => setDecision('approved'),
    })

    const deny = useMutation({
        mutationFn: async () => {
            const body = new FormData()
            body.append('user_code', code)
            await pb.send('/oauth/authorize/deny', { method: 'POST', body })
        },
        onSuccess: () => setDecision('denied'),
    })

    return { decision, approve, deny }
}

export default function OAuthAuthorizeScreen() {
    const params = useLocalSearchParams<{ user_code?: string }>()
    const auth = useAuth({ throwIfAnon: false })
    const [code, setCode] = useState((params.user_code ?? '').toUpperCase())
    const [deviceLabel, setDeviceLabel] = useState('')

    const info = useAuthorizeInfo(code, auth.isLoggedIn)
    const { decision, approve, deny } = useDeviceDecision(code)

    return (
        <View className="flex-1 bg-background">
            <DocumentTitle title="Connect a device" includeOrg={false} />
            <SignInPrompt isVisible={!auth.isLoggedIn && !auth.isInitializing} />
            <DecisionResult decision={decision} />
            <ConsentForm
                isVisible={auth.isLoggedIn && decision === null}
                code={code}
                onChangeCode={setCode}
                deviceLabel={deviceLabel}
                onChangeDeviceLabel={setDeviceLabel}
                info={info}
                onApprove={() => approve.mutate(deviceLabel || 'Unnamed device')}
                onDeny={() => deny.mutate()}
                isApproving={approve.isPending}
                isDenying={deny.isPending}
            />
        </View>
    )
}

function SignInPrompt({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    // AuthGate renders the same sign-in modal used everywhere else in the app;
    // once auth succeeds this screen re-renders with isLoggedIn true and the
    // consent form takes over — no separate /login route to bounce through.
    return <AuthGate />
}

function DecisionResult({ decision }: { decision: Decision }) {
    if (decision === null) return null

    const title = decision === 'approved' ? 'Device connected' : 'Request denied'
    const description =
        decision === 'approved'
            ? 'You can return to your terminal.'
            : 'Nothing was granted access. You can close this page.'

    return (
        <View className="flex-1 items-center justify-center gap-2 p-6">
            <Text className="text-foreground text-xl font-semibold">{title}</Text>
            <Text className="text-muted-foreground text-center">{description}</Text>
        </View>
    )
}

interface ConsentFormProps {
    isVisible: boolean
    code: string
    onChangeCode: (code: string) => void
    deviceLabel: string
    onChangeDeviceLabel: (label: string) => void
    info: ReturnType<typeof useAuthorizeInfo>
    onApprove: () => void
    onDeny: () => void
    isApproving: boolean
    isDenying: boolean
}

function ConsentForm({
    isVisible,
    code,
    onChangeCode,
    deviceLabel,
    onChangeDeviceLabel,
    info,
    onApprove,
    onDeny,
    isApproving,
    isDenying,
}: ConsentFormProps) {
    if (!isVisible) return null

    return (
        <View
            className="flex-1 gap-6 p-6"
            style={{ maxWidth: 480, width: '100%', alignSelf: 'center' }}
        >
            <Text className="text-foreground text-2xl font-semibold">Connect a device</Text>
            <CodeInput code={code} onChangeCode={onChangeCode} />
            <InfoStatus info={info} />
            <GrantDetails
                info={info.data}
                deviceLabel={deviceLabel}
                onChangeDeviceLabel={onChangeDeviceLabel}
                onApprove={onApprove}
                onDeny={onDeny}
                isApproving={isApproving}
                isDenying={isDenying}
            />
        </View>
    )
}

function CodeInput({ code, onChangeCode }: { code: string; onChangeCode: (code: string) => void }) {
    const placeholderColor = useThemeColor('field-placeholder')

    return (
        <View className="gap-2">
            <Text className="text-muted-foreground">Enter the code shown in your terminal</Text>
            <TextInput
                value={code}
                onChangeText={text => onChangeCode(text.toUpperCase())}
                placeholder="WDJB-MJHT"
                placeholderTextColor={placeholderColor}
                autoCapitalize="characters"
                autoCorrect={false}
                className="border border-border rounded-lg px-3 py-2.5 text-base text-foreground bg-background"
            />
        </View>
    )
}

function InfoStatus({ info }: { info: ReturnType<typeof useAuthorizeInfo> }) {
    if (info.isLoading) {
        return (
            <View className="items-center py-4">
                <ActivityIndicator />
            </View>
        )
    }

    if (info.isError) {
        return (
            <Text className="text-destructive">
                That code is not valid, has expired, or has already been used.
            </Text>
        )
    }

    return null
}

function GrantDetails({
    info,
    deviceLabel,
    onChangeDeviceLabel,
    onApprove,
    onDeny,
    isApproving,
    isDenying,
}: {
    info: AuthorizeInfo | undefined
    deviceLabel: string
    onChangeDeviceLabel: (label: string) => void
    onApprove: () => void
    onDeny: () => void
    isApproving: boolean
    isDenying: boolean
}) {
    const placeholderColor = useThemeColor('field-placeholder')

    if (!info) return null

    return (
        <View className="gap-4">
            <Text className="text-foreground text-lg">{info.client_name} wants access to:</Text>
            <ScopeList scopes={info.scopes} />

            <View className="gap-2">
                <Text className="text-muted-foreground">Name this device</Text>
                <TextInput
                    value={deviceLabel}
                    onChangeText={onChangeDeviceLabel}
                    placeholder="My laptop"
                    placeholderTextColor={placeholderColor}
                    className="border border-border rounded-lg px-3 py-2.5 text-base text-foreground bg-background"
                />
            </View>

            <View className="flex-row gap-3">
                <Button onPress={onApprove} disabled={isApproving || isDenying} className="flex-1">
                    {isApproving ? <ActivityIndicator /> : <ButtonText>Connect</ButtonText>}
                </Button>
                <Button
                    variant="outline"
                    onPress={onDeny}
                    disabled={isApproving || isDenying}
                    className="flex-1"
                >
                    {isDenying ? <ActivityIndicator /> : <ButtonText>Deny</ButtonText>}
                </Button>
            </View>
        </View>
    )
}
