import { useAuthStore } from '@tinycld/core/lib/stores/auth-store'
import { useState } from 'react'
import { ActivityIndicator, Pressable, Text, TextInput, View } from 'react-native'

export interface ShareLinkSignInProps {
    /** The share link token (the public token from /share/[token]). */
    token: string
    /** The share role — affects copy ("comment" vs "edit"). */
    role: 'commentor' | 'editor'
    /** Called after a successful verify (the auth store is already updated). */
    onSuccess: () => void
}

// Two-step email-OTP flow for guest provisioning on a share link.
// Step 1: collect email, POST /otp-request, advance on success.
// Step 2: collect the 6-digit code, POST /otp-verify; pb.authStore is
// updated by the verify call, then onSuccess fires.
//
// All server error messages are surfaced verbatim — including the
// uniform "invalid or expired code" used by the verify endpoint (which
// deliberately doesn't distinguish wrong-code from expired/unknown so
// brute-forcing can't enumerate).
export function ShareLinkSignIn({ token, role, onSuccess }: ShareLinkSignInProps) {
    const requestShareOtp = useAuthStore(s => s.requestShareOtp)
    const verifyShareOtp = useAuthStore(s => s.verifyShareOtp)

    const [step, setStep] = useState<'email' | 'code'>('email')
    const [email, setEmail] = useState('')
    const [otpId, setOtpId] = useState('')
    const [code, setCode] = useState('')
    const [submitting, setSubmitting] = useState(false)
    const [error, setError] = useState<string | null>(null)

    const verb = role === 'editor' ? 'edit' : 'comment on'

    const submitEmail = async () => {
        setSubmitting(true)
        setError(null)
        const { otpId: id, error: err } = await requestShareOtp(token, email.trim())
        setSubmitting(false)
        if (err || !id) {
            setError(err ?? 'failed to send code')
            return
        }
        setOtpId(id)
        setStep('code')
    }

    const submitCode = async () => {
        setSubmitting(true)
        setError(null)
        const { user, error: err } = await verifyShareOtp(token, email.trim(), code.trim(), otpId)
        setSubmitting(false)
        if (err || !user) {
            setError(err ?? 'failed to verify code')
            return
        }
        onSuccess()
    }

    if (step === 'email') {
        return (
            <View className="gap-3 p-6 max-w-md w-full self-center">
                <Text className="text-foreground" style={{ fontSize: 18, fontWeight: '600' }}>
                    {`Sign in to ${verb} this document`}
                </Text>
                <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                    {`We'll email you a one-time code. No password required.`}
                </Text>
                <TextInput
                    value={email}
                    onChangeText={setEmail}
                    placeholder="your@email"
                    autoCapitalize="none"
                    keyboardType="email-address"
                    autoComplete="email"
                    editable={!submitting}
                    className="border border-border rounded-md px-3 py-2 text-foreground"
                />
                {error && (
                    <Text className="text-red-600" style={{ fontSize: 13 }}>
                        {error}
                    </Text>
                )}
                <Pressable
                    onPress={submitEmail}
                    disabled={submitting || !email.trim()}
                    className="bg-primary px-4 py-2 rounded-md items-center"
                >
                    {submitting ? (
                        <ActivityIndicator />
                    ) : (
                        <Text className="text-primary-foreground">Send code</Text>
                    )}
                </Pressable>
            </View>
        )
    }

    return (
        <View className="gap-3 p-6 max-w-md w-full self-center">
            <Text className="text-foreground" style={{ fontSize: 18, fontWeight: '600' }}>
                Enter your code
            </Text>
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                {`We sent a code to ${email}.`}
            </Text>
            <TextInput
                value={code}
                onChangeText={setCode}
                placeholder="123456"
                keyboardType="number-pad"
                editable={!submitting}
                className="border border-border rounded-md px-3 py-2 text-foreground text-center"
                style={{ fontSize: 18, letterSpacing: 4 }}
            />
            {error && (
                <Text className="text-red-600" style={{ fontSize: 13 }}>
                    {error}
                </Text>
            )}
            <Pressable
                onPress={submitCode}
                disabled={submitting || !code.trim()}
                className="bg-primary px-4 py-2 rounded-md items-center"
            >
                {submitting ? (
                    <ActivityIndicator />
                ) : (
                    <Text className="text-primary-foreground">Verify</Text>
                )}
            </Pressable>
            <Pressable
                onPress={() => {
                    setStep('email')
                    setCode('')
                    setError(null)
                }}
                disabled={submitting}
            >
                <Text
                    className="text-muted-foreground"
                    style={{ fontSize: 13, textAlign: 'center' }}
                >
                    Use a different email
                </Text>
            </Pressable>
        </View>
    )
}
