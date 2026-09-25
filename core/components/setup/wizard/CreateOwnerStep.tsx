import { useQueryClient } from '@tanstack/react-query'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { appHref } from '@tinycld/core/lib/org-routes'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { NEEDS_SETUP_QUERY_KEY } from '@tinycld/core/lib/setup/use-needs-setup'
import { useAuthStore } from '@tinycld/core/lib/stores/auth-store'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { type Control, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useRouter } from 'expo-router'
import { useState } from 'react'
import { Platform, Pressable, Text, View } from 'react-native'
import { claimErrorMessage, refusalBodyOf } from './ClaimServerStep'
import { postSetup, SetupRequestError } from './setup-api'
import { initialsOf } from './use-workspace-preview'

const ownerSchema = z
    .object({
        name: z.string().trim().min(1, 'Enter your name').max(255),
        email: z.string().email('Enter a valid email address'),
        password: z.string().min(10, 'Use at least 10 characters'),
        confirmPassword: z.string(),
        appUrl: z.string().url('Enter a full web address, such as https://cloud.example.com'),
    })
    .refine(data => data.password === data.confirmPassword, {
        message: 'The passwords do not match',
        path: ['confirmPassword'],
    })

type OwnerForm = z.infer<typeof ownerSchema>

interface InitResult {
    authToken: string
    email: string
    userId: string
}

function defaultAppUrl(): string {
    // On web the app is served same-origin, so window.location.origin is the app
    // URL. On native there is no window.location (RN defines a partial `window`
    // WITHOUT `location`, so a `typeof window` check wrongly takes the web branch
    // and throws "Cannot read property 'origin' of undefined") — use the resolved
    // server address instead. getResolvedAddress() may be null pre-connect; fall
    // back to '' so the field is simply empty rather than crashing render.
    return Platform.OS === 'web' ? window.location.origin : (getResolvedAddress() ?? '')
}

function useCreateOwner(code: string, onCodeRejected: (message: string) => void) {
    const router = useRouter()
    const queryClient = useQueryClient()
    const signInWithToken = useAuthStore(s => s.signInWithToken)
    const form = useForm<OwnerForm>({
        resolver: zodResolver(ownerSchema),
        defaultValues: {
            name: '',
            email: '',
            password: '',
            confirmPassword: '',
            appUrl: defaultAppUrl(),
        },
    })
    const reportOther = handleMutationErrorsWithForm({
        setError: form.setError,
        getValues: form.getValues,
        operation: 'setup.init',
    })

    const create = useMutation({
        mutationFn: async (data: OwnerForm) => {
            const result = await postSetup<InitResult>('init', {
                code,
                name: data.name.trim(),
                email: data.email,
                password: data.password,
                appUrl: data.appUrl,
            })
            // signInWithToken, not a bare authStore.save: it also refetches
            // the stores these signed-out screens synced as nobody.
            const signedIn = await signInWithToken(result.authToken, {
                id: result.userId,
                email: result.email,
            })
            if (signedIn.error) throw new Error(signedIn.error)
        },
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: NEEDS_SETUP_QUERY_KEY })
            router.replace(appHref('setup/next'))
        },
        onError: error => {
            if (error instanceof SetupRequestError && error.status === 403) {
                onCodeRejected(claimErrorMessage(refusalBodyOf(error)))
                return
            }
            if (error instanceof SetupRequestError && error.status === null) {
                form.setError('root', { type: 'server', message: claimErrorMessage(null) })
                return
            }
            reportOther(error)
        },
    })

    return {
        form,
        onSubmit: form.handleSubmit(data => create.mutate(data)),
        isPending: create.isPending,
    }
}

function AdvancedToggle({ isOpen, onPress }: { isOpen: boolean; onPress: () => void }) {
    const label = isOpen ? '▾ Advanced' : '▸ Advanced'
    return (
        <Pressable
            onPress={onPress}
            accessibilityRole="button"
            accessibilityState={{ expanded: isOpen }}
            className="self-start py-1"
        >
            <Text className="text-xs text-muted-foreground">{label}</Text>
        </Pressable>
    )
}

// Unmounting the field keeps its value: react-hook-form does not unregister by default.
function AdvancedFields({
    control,
    isVisible,
}: {
    control: Control<OwnerForm>
    isVisible: boolean
}) {
    if (!isVisible) return null
    return (
        <TextInput
            control={control}
            name="appUrl"
            label="Web address"
            autoCapitalize="none"
            keyboardType="url"
            hint="The address people use to open this server."
        />
    )
}

function RootError({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <View className="rounded-lg bg-danger-soft p-2.5">
            <Text className="text-sm text-danger">{message}</Text>
        </View>
    )
}

/** Creates the owner, signs them in and hands over to the signed-in wizard. */
export function CreateOwnerStep({
    code,
    onNameChange,
    onCodeRejected,
}: {
    code: string
    onNameChange: (initials: string) => void
    onCodeRejected: (message: string) => void
}) {
    const { form, onSubmit, isPending } = useCreateOwner(code, onCodeRejected)
    const [isAdvancedOpen, setAdvancedOpen] = useState(false)
    const { control } = form
    // An error in a hidden field would block the submit with no visible reason.
    const showAdvanced = isAdvancedOpen || !!form.formState.errors.appUrl
    return (
        <View className="max-w-[440px] gap-1">
            <Text className="text-2xl font-bold text-foreground">Create your owner account</Text>
            <Text className="mb-3 text-sm text-muted-foreground">
                You manage this server and everyone on it.
            </Text>
            <RootError message={form.formState.errors.root?.message} />
            <TextInput
                control={control}
                name="name"
                label="Name"
                autoComplete="name"
                textContentType="name"
                onValueChange={value => onNameChange(initialsOf(value))}
            />
            <TextInput
                control={control}
                name="email"
                label="Email"
                keyboardType="email-address"
                autoCapitalize="none"
                autoComplete="email"
                textContentType="emailAddress"
            />
            <TextInput
                control={control}
                name="password"
                label="Password"
                placeholder="At least 10 characters"
                secureTextEntry
                autoComplete="new-password"
                textContentType="newPassword"
            />
            <TextInput
                control={control}
                name="confirmPassword"
                label="Confirm password"
                secureTextEntry
                autoComplete="new-password"
                textContentType="newPassword"
            />
            <AdvancedToggle isOpen={showAdvanced} onPress={() => setAdvancedOpen(o => !o)} />
            <AdvancedFields control={control} isVisible={showAdvanced} />
            <Button className="mt-2 self-start" onPress={onSubmit} isDisabled={isPending}>
                <ButtonText>Create account</ButtonText>
            </Button>
        </View>
    )
}
