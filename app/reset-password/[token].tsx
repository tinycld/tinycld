import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { confirmPasswordReset } from '@tinycld/core/lib/account-password'
import { captureException } from '@tinycld/core/lib/errors'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { CheckCircle2, KeyRound } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

const resetSchema = z
    .object({
        password: z.string().min(8, 'Min 8 characters'),
        confirmPassword: z.string(),
    })
    .refine(v => v.password === v.confirmPassword, {
        message: 'Passwords must match',
        path: ['confirmPassword'],
    })

export default function ResetPassword() {
    const { token } = useLocalSearchParams<{ token: string }>()

    return (
        <View className="flex-1 items-center justify-center p-5 bg-background">
            <DocumentTitle title="Reset password" includeOrg={false} />
            {token ? <ResetForm token={token} /> : <InvalidCard />}
        </View>
    )
}

function InvalidCard() {
    const router = useRouter()

    return (
        <View
            className="gap-4 p-6 rounded-xl border border-border items-center bg-surface-secondary"
            style={{ maxWidth: 400, width: '100%' }}
        >
            <Text className="text-lg font-semibold text-foreground">Reset link invalid</Text>
            <Text className="text-center text-sm text-muted-foreground">
                This password reset link is missing or malformed. Request a new one from the sign-in
                screen.
            </Text>
            <Pressable
                onPress={() => router.replace('/')}
                className="px-4 py-2 rounded-lg bg-primary"
            >
                <Text className="font-semibold text-primary-foreground">Go to sign in</Text>
            </Pressable>
        </View>
    )
}

function ResetForm({ token }: { token: string }) {
    const router = useRouter()
    const fgColor = useThemeColor('foreground')
    const primaryBg = useThemeColor('primary')

    const [submitError, setSubmitError] = useState<string | null>(null)
    const [isSubmitting, setIsSubmitting] = useState(false)
    const [isSuccess, setIsSuccess] = useState(false)

    const {
        control,
        handleSubmit,
        formState: { errors, isSubmitted },
    } = useForm({
        resolver: zodResolver(resetSchema),
        defaultValues: { password: '', confirmPassword: '' },
        mode: 'onChange',
    })

    const onSubmit = handleSubmit(async data => {
        setSubmitError(null)
        setIsSubmitting(true)
        try {
            await confirmPasswordReset(token, data.password, data.confirmPassword)
            setIsSuccess(true)
            router.replace('/')
        } catch (err) {
            captureException('account.confirmPasswordReset', err)
            setSubmitError(
                'This reset link is invalid or has expired. Request a new one from the sign-in screen.'
            )
        } finally {
            setIsSubmitting(false)
        }
    })

    return (
        <View
            className="gap-4 p-6 rounded-xl border border-border bg-surface-secondary"
            style={{ maxWidth: 440, width: '100%' }}
        >
            <View className="gap-2 items-center">
                <View className="size-10 rounded-lg items-center justify-center bg-surface">
                    {isSuccess ? (
                        <CheckCircle2 size={20} color={primaryBg} />
                    ) : (
                        <KeyRound size={18} color={fgColor} />
                    )}
                </View>
                <Text className="text-xl font-semibold text-foreground text-center">
                    Choose a new password
                </Text>
                <Text
                    className="text-center text-[13px] text-muted-foreground"
                    style={{ lineHeight: 18 }}
                >
                    Enter a new password for your account.
                </Text>
            </View>

            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            {submitError && (
                <View className="rounded-lg p-2 bg-danger-soft" testID="reset-error">
                    <Text className="text-xs text-danger">{submitError}</Text>
                </View>
            )}

            <TextInput
                control={control}
                name="password"
                label="New password"
                placeholder="At least 8 characters"
                secureTextEntry
                autoCapitalize="none"
            />

            <TextInput
                control={control}
                name="confirmPassword"
                label="Confirm password"
                placeholder="Re-enter password"
                secureTextEntry
                autoCapitalize="none"
            />

            <Pressable
                testID="reset-confirm-submit"
                onPress={onSubmit}
                disabled={isSubmitting || isSuccess}
                className={`px-4 py-3 rounded-lg items-center bg-primary ${isSubmitting || isSuccess ? 'opacity-60' : 'opacity-100'}`}
            >
                <Text className="font-semibold text-primary-foreground">
                    {isSuccess
                        ? 'Password reset, redirecting...'
                        : isSubmitting
                          ? 'Resetting...'
                          : 'Reset password'}
                </Text>
            </Pressable>
        </View>
    )
}
