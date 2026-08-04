import { ChangeServerLink } from '@tinycld/core/components/ChangeServerLink'
import { ReviewModeHints } from '@tinycld/core/components/connect/ReviewModeHints'
import { requestPasswordReset } from '@tinycld/core/lib/account-password'
import { useAuth } from '@tinycld/core/lib/auth'
import { navigateToOrg } from '@tinycld/core/lib/org-url'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useState } from 'react'
import {
    ActivityIndicator,
    KeyboardAvoidingView,
    Platform,
    Pressable,
    Text,
    TextInput,
    View,
} from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'

type Mode = 'login' | 'reset'

export function LoginModal() {
    const backdropColor = useThemeColor('overlay-backdrop')
    const [mode, setMode] = useState<Mode>('login')
    const insets = useSafeAreaInsets()

    return (
        <KeyboardAvoidingView
            behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
            className="absolute top-0 left-0 right-0 bottom-0 justify-center items-center"
            style={{
                zIndex: 200,
                backgroundColor: backdropColor,
                // The backdrop deliberately still covers the full screen — only
                // the card is inset. In landscape the sensor housing sits on one
                // side (~59pt), so a centred card can otherwise have its edge
                // clipped by the notch on a narrow window.
                paddingLeft: insets.left,
                paddingRight: insets.right,
                paddingTop: insets.top,
                paddingBottom: insets.bottom,
            }}
        >
            <View
                className="rounded-2xl border border-border p-8 bg-background"
                style={{
                    width: 400,
                    maxWidth: '90%',
                    shadowColor: '#000',
                    shadowOffset: { width: 0, height: 8 },
                    shadowOpacity: 0.15,
                    shadowRadius: 24,
                    elevation: 8,
                }}
            >
                <LoginForm isVisible={mode === 'login'} onForgotPassword={() => setMode('reset')} />
                <ResetForm isVisible={mode === 'reset'} onBack={() => setMode('login')} />
            </View>
        </KeyboardAvoidingView>
    )
}

function LoginForm({
    isVisible,
    onForgotPassword,
}: {
    isVisible: boolean
    onForgotPassword: () => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const primaryFg = useThemeColor('primary-foreground')
    const { login } = useAuth({ throwIfAnon: false })
    const [identifier, setIdentifier] = useState('')
    const [password, setPassword] = useState('')
    const [error, setError] = useState<string | null>(null)
    const [isSubmitting, setIsSubmitting] = useState(false)

    const canSubmit = identifier.trim().length > 0 && password.length > 0 && !isSubmitting

    const handleSubmit = async () => {
        if (!canSubmit) return
        setError(null)
        setIsSubmitting(true)
        const result = await login(identifier.trim(), password)
        if (result.error) {
            setError(result.error)
            setIsSubmitting(false)
        } else if (result.user) {
            // Single-org deployment: org identity comes from the deployment, not
            // the user — send every signed-in user to the app root.
            navigateToOrg()
        }
    }

    if (!isVisible) return null

    return (
        <>
            <Text className="text-[22px] font-bold mb-1 text-foreground">Sign in</Text>
            <Text className="text-sm mb-6 text-muted-foreground">
                Sign in to your account to continue
            </Text>

            {error && (
                <View className="rounded-lg p-3 mb-4 bg-danger-soft">
                    <Text className="text-sm text-danger">{error}</Text>
                </View>
            )}

            <View className="mb-4">
                <Text className="mb-1.5 text-sm font-semibold text-foreground">
                    Username or email
                </Text>
                <TextInput
                    className="border border-border rounded-lg p-3 text-base text-foreground bg-surface-secondary"
                    testID="identifier"
                    value={identifier}
                    onChangeText={setIdentifier}
                    placeholder="alice or alice@company.com"
                    placeholderTextColor={mutedColor}
                    autoCapitalize="none"
                    autoCorrect={false}
                    autoComplete="username"
                    editable={!isSubmitting}
                />
            </View>

            <View className="mb-2">
                <Text className="mb-1.5 text-sm font-semibold text-foreground">Password</Text>
                <TextInput
                    className="border border-border rounded-lg p-3 text-base text-foreground bg-surface-secondary"
                    testID="login-password"
                    value={password}
                    onChangeText={setPassword}
                    placeholder="Password"
                    placeholderTextColor={mutedColor}
                    secureTextEntry
                    autoComplete="current-password"
                    editable={!isSubmitting}
                    onSubmitEditing={handleSubmit}
                />
            </View>

            <Pressable
                testID="login-submit"
                className={`rounded-lg items-center mt-4 p-3.5 bg-primary ${canSubmit ? 'opacity-100' : 'opacity-50'}`}
                onPress={handleSubmit}
                disabled={!canSubmit}
            >
                {isSubmitting ? (
                    <ActivityIndicator color={primaryFg} size="small" />
                ) : (
                    <Text className="text-base font-semibold text-primary-foreground">Sign in</Text>
                )}
            </Pressable>

            <ReviewModeHints
                onPrefill={(id, password) => {
                    setIdentifier(id)
                    setPassword(password)
                }}
            />

            <View className="mt-4 flex-row justify-around">
                <Pressable
                    testID="forgot-password-link"
                    accessibilityRole="link"
                    onPress={onForgotPassword}
                >
                    <Text className="text-xs text-muted-foreground underline">
                        Forgot password?
                    </Text>
                </Pressable>
                <ChangeServerLink />
            </View>
        </>
    )
}

function ResetForm({ isVisible, onBack }: { isVisible: boolean; onBack: () => void }) {
    const mutedColor = useThemeColor('muted-foreground')
    const primaryFg = useThemeColor('primary-foreground')
    const [email, setEmail] = useState('')
    const [isSubmitting, setIsSubmitting] = useState(false)
    const [isSent, setIsSent] = useState(false)

    const canSubmit = email.trim().length > 0 && !isSubmitting

    const handleSubmit = async () => {
        if (!canSubmit) return
        setIsSubmitting(true)
        // requestPasswordReset never throws and never reveals whether the email
        // exists — we always land on the same confirmation.
        await requestPasswordReset(email.trim())
        setIsSubmitting(false)
        setIsSent(true)
    }

    if (!isVisible) return null

    return (
        <>
            <Text className="text-[22px] font-bold mb-1 text-foreground">Reset password</Text>
            <Text className="text-sm mb-6 text-muted-foreground">
                Enter your email and we'll send you a reset link
            </Text>

            {isSent ? (
                <View className="rounded-lg p-3 mb-4 bg-surface-secondary" testID="reset-sent">
                    <Text className="text-sm text-foreground">
                        If an account exists for that email, we've sent a reset link. Check your
                        inbox.
                    </Text>
                </View>
            ) : (
                <View className="mb-2">
                    <Text className="mb-1.5 text-sm font-semibold text-foreground">Email</Text>
                    <TextInput
                        className="border border-border rounded-lg p-3 text-base text-foreground bg-surface-secondary"
                        testID="reset-email"
                        value={email}
                        onChangeText={setEmail}
                        placeholder="alice@company.com"
                        placeholderTextColor={mutedColor}
                        autoCapitalize="none"
                        autoCorrect={false}
                        keyboardType="email-address"
                        autoComplete="email"
                        editable={!isSubmitting}
                        onSubmitEditing={handleSubmit}
                    />
                </View>
            )}

            {!isSent && (
                <Pressable
                    testID="reset-submit"
                    className={`rounded-lg items-center mt-2 p-3.5 bg-primary ${canSubmit ? 'opacity-100' : 'opacity-50'}`}
                    onPress={handleSubmit}
                    disabled={!canSubmit}
                >
                    {isSubmitting ? (
                        <ActivityIndicator color={primaryFg} size="small" />
                    ) : (
                        <Text className="text-base font-semibold text-primary-foreground">
                            Send reset link
                        </Text>
                    )}
                </Pressable>
            )}

            <Pressable
                testID="reset-back"
                accessibilityRole="link"
                onPress={onBack}
                className="mt-4 items-center"
            >
                <Text className="text-xs text-muted-foreground underline">Back to sign in</Text>
            </Pressable>
        </>
    )
}
