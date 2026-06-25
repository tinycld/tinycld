import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import PocketBase from 'pocketbase'
import { useRef, useState } from 'react'
import { Platform, Pressable, ScrollView, Text, View } from 'react-native'
import { GestureHandlerRootView } from 'react-native-gesture-handler'
import { SetupDashboard } from './SetupDashboard'

const bootstrapSchema = z
    .object({
        appName: z.string().min(1, 'App name is required').max(255),
        email: z.string().email(),
        password: z.string().min(10, 'Min 10 characters'),
        confirmPassword: z.string(),
        appUrl: z.string().url('Must be a valid URL'),
    })
    .refine(data => data.password === data.confirmPassword, {
        message: 'Passwords do not match',
        path: ['confirmPassword'],
    })

interface SetupWizardProps {
    token: string
}

export function SetupWizard({ token }: SetupWizardProps) {
    const pbRef = useRef<PocketBase | null>(null)
    if (!pbRef.current) {
        pbRef.current = new PocketBase(PB_SERVER_ADDR)
    }
    const pb = pbRef.current

    const [isComplete, setIsComplete] = useState(false)
    const [submitError, setSubmitError] = useState<string | null>(null)
    const [isSubmitting, setIsSubmitting] = useState(false)

    // On web the app is served same-origin, so window.location.origin is the app
    // URL. On native there is no window.location (RN defines a partial `window`
    // WITHOUT `location`, so a `typeof window` check wrongly takes the web branch
    // and throws "Cannot read property 'origin' of undefined") — use the resolved
    // server address instead. getResolvedAddress() may be null pre-connect; fall
    // back to '' so the field is simply empty rather than crashing render.
    const defaultAppUrl =
        Platform.OS === 'web' ? window.location.origin : (getResolvedAddress() ?? '')

    const {
        control,
        handleSubmit,
        formState: { errors, isSubmitted },
    } = useForm({
        resolver: zodResolver(bootstrapSchema),
        defaultValues: {
            appName: 'tinycld',
            email: '',
            password: '',
            confirmPassword: '',
            appUrl: defaultAppUrl,
        },
        mode: 'onChange',
    })

    const onSubmit = handleSubmit(async data => {
        setSubmitError(null)
        setIsSubmitting(true)
        try {
            const res = await fetch(`${PB_SERVER_ADDR}/api/setup/init`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    token,
                    appName: data.appName,
                    email: data.email,
                    password: data.password,
                    appUrl: data.appUrl,
                }),
            })
            const result = await res.json()
            if (!res.ok) {
                setSubmitError(result.error ?? 'Setup failed')
                return
            }
            pb.authStore.save(result.authToken, {
                id: '',
                email: result.email,
                collectionId: '_superusers',
                collectionName: '_superusers',
            })
            setIsComplete(true)
        } catch (err) {
            setSubmitError(err instanceof Error ? err.message : 'Network error')
        } finally {
            setIsSubmitting(false)
        }
    })

    if (isComplete) {
        return (
            <GestureHandlerRootView className="flex-1">
                <ScrollView>
                    <SetupDashboard pb={pb} defaultTab="organizations" />
                </ScrollView>
            </GestureHandlerRootView>
        )
    }

    return (
        <View className="flex-1 items-center justify-center py-12">
            <View
                className="gap-4 p-5 self-center rounded-xl border border-border bg-surface-secondary"
                style={{ maxWidth: 420, width: '90%' }}
            >
                <View className="gap-2 items-center">
                    <View className="size-10 rounded-lg items-center justify-center bg-surface">
                        <Text className="text-xl">&#9889;</Text>
                    </View>
                    <Text className="text-xl font-bold text-foreground">Welcome to TinyCld</Text>
                    <Text className="text-center text-xs text-muted-foreground">
                        Create a superuser account to get started.
                    </Text>
                </View>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                {submitError && (
                    <View className="rounded-lg p-2 bg-danger-soft">
                        <Text className="text-xs text-danger">{submitError}</Text>
                    </View>
                )}

                <TextInput
                    control={control}
                    name="appName"
                    label="Application Name"
                    placeholder="tinycld"
                />

                <TextInput
                    control={control}
                    name="email"
                    label="Email"
                    placeholder="admin@example.com"
                    keyboardType="email-address"
                    autoCapitalize="none"
                />

                <TextInput
                    control={control}
                    name="password"
                    label="Password"
                    placeholder="At least 10 characters"
                    secureTextEntry
                />

                <TextInput
                    control={control}
                    name="confirmPassword"
                    label="Confirm Password"
                    placeholder="Repeat password"
                    secureTextEntry
                />

                <TextInput
                    control={control}
                    name="appUrl"
                    label="App URL"
                    placeholder="https://your-domain.com"
                    autoCapitalize="none"
                    hint="The public URL where this instance is accessible"
                />

                <Pressable
                    onPress={onSubmit}
                    disabled={isSubmitting}
                    accessibilityRole="button"
                    className={`px-4 py-3 rounded-lg items-center bg-primary ${isSubmitting ? 'opacity-60' : 'opacity-100'}`}
                >
                    <Text className="font-semibold text-primary-foreground">
                        {isSubmitting ? 'Setting up...' : 'Create Account & Continue'}
                    </Text>
                </Pressable>
            </View>
        </View>
    )
}
