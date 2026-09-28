import { useQueryClient } from '@tanstack/react-query'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { appHref } from '@tinycld/core/lib/org-routes'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { NEEDS_SETUP_QUERY_KEY } from '@tinycld/core/lib/setup/use-needs-setup'
import { useAuthStore } from '@tinycld/core/lib/stores/auth-store'
import { TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useRouter } from 'expo-router'
import { Platform, Text, View } from 'react-native'
import { claimErrorMessage, refusalBodyOf } from './ClaimServerStep'
import { SetupContinueButton } from './SetupContinueButton'
import { StepHeading } from './StepHeading'
import { ownerFailureOf, postSetup } from './setup-api'

const ownerSchema = z
    .object({
        name: z.string().trim().min(1, 'Enter your name').max(255),
        email: z.string().email('Enter a valid email address'),
        password: z.string().min(8, 'Use at least 8 characters'),
        confirmPassword: z.string(),
    })
    .refine(data => data.password === data.confirmPassword, {
        message: 'The passwords do not match',
        path: ['confirmPassword'],
    })

type OwnerForm = z.infer<typeof ownerSchema>

const SETUP_NEXT_HREF = appHref('setup/next')

interface InitResult {
    authToken: string
    email: string
    userId: string
}

// The address invite and password-reset emails link to. It is not asked
// for: on web it is the origin this page was opened from, which is the address
// people will keep using, and a deployment that fronts the server with another
// hostname sets TINYCLD_PUBLIC_URL, which overrides it on every boot.
function appUrl(): string {
    // On native there is no window.location (RN defines a partial `window`
    // WITHOUT `location`, so a `typeof window` check wrongly takes the web branch
    // and throws "Cannot read property 'origin' of undefined") — use the resolved
    // server address instead. getResolvedAddress() may be null pre-connect; an
    // empty value leaves the server's own setting untouched.
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
                appUrl: appUrl(),
            })
            // The owner exists from here on, whether or not the sign-in below
            // works, so the claim screens must not come back.
            await queryClient.invalidateQueries({ queryKey: NEEDS_SETUP_QUERY_KEY })
            // signInWithToken, not a bare authStore.save: it also refetches
            // the stores these signed-out screens synced as nobody. A failure
            // is logged there; the wizard route then asks the person to sign
            // in with the account they just created.
            await signInWithToken(result.authToken, {
                id: result.userId,
                email: result.email,
            })
        },
        // Signed in: the wizard. Not signed in: that route shows sign-in.
        onSuccess: () => router.replace(SETUP_NEXT_HREF),
        onError: async error => {
            const failure = ownerFailureOf(error)
            if (failure === 'sign-in') {
                await queryClient.invalidateQueries({ queryKey: NEEDS_SETUP_QUERY_KEY })
                router.replace(SETUP_NEXT_HREF)
                return
            }
            if (failure === 'code-rejected') {
                onCodeRejected(claimErrorMessage(refusalBodyOf(error)))
                return
            }
            if (failure === 'offline') {
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

function RootError({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <View className="mb-4 rounded-lg border border-danger/30 bg-danger-soft px-3.5 py-3">
            <Text className="text-sm text-danger">{message}</Text>
        </View>
    )
}

/** Creates the owner, signs them in and hands over to the signed-in wizard. */
export function CreateOwnerStep({
    code,
    onCodeRejected,
}: {
    code: string
    onCodeRejected: (message: string) => void
}) {
    const { form, onSubmit, isPending } = useCreateOwner(code, onCodeRejected)
    const { control } = form
    return (
        <View>
            <StepHeading
                title="Create your owner account"
                lead="This account is the owner of the server. It can change every setting, manage apps and people, and hand the owner role to someone else later. Use the name and email you sign in with."
            />
            <RootError message={form.formState.errors.root?.message} />
            <TextInput
                control={control}
                name="name"
                label="Name"
                autoComplete="name"
                textContentType="name"
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
                placeholder="At least 8 characters"
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
            <SetupContinueButton onPress={onSubmit} isDisabled={isPending} label="Create account" />
        </View>
    )
}
