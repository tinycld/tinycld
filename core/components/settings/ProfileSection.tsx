import { AvatarSection } from '@tinycld/core/components/settings/AvatarSection'
import { SectionCard } from '@tinycld/core/components/settings/SectionCard'
import { changeMyPassword } from '@tinycld/core/lib/account-password'
import { useAuth } from '@tinycld/core/lib/auth'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useState } from 'react'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'

const profileSchema = z.object({
    name: z.string().min(1, 'Name is required'),
    email: z.string().email('Valid email is required'),
})

const passwordSchema = z
    .object({
        oldPassword: z.string().min(1, 'Current password is required'),
        password: z.string().min(8, 'New password must be at least 8 characters'),
        passwordConfirm: z.string().min(1, 'Please confirm your new password'),
    })
    .refine(data => data.password === data.passwordConfirm, {
        path: ['passwordConfirm'],
        message: 'Passwords do not match',
    })

export function ProfileSection() {
    const { user } = useAuth()
    const [usersCollection] = useStore('users')

    const {
        control,
        setError,
        getValues,
        handleSubmit,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(profileSchema),
        values: { name: user.name, email: user.email },
    })

    const updateProfile = useMutation({
        mutationFn: mutation(function* (data: z.infer<typeof profileSchema>) {
            yield usersCollection.update(user.id, draft => {
                draft.name = data.name.trim()
                draft.email = data.email.trim()
            })
        }),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const saveIfValid = handleSubmit(data => {
        if (!isDirty) return
        updateProfile.mutate(data)
    })

    return (
        <View className="gap-3">
            <AvatarSection />

            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            <View className="gap-4">
                <TextInput control={control} name="name" label="Name" onBlur={saveIfValid} />
                <TextInput
                    control={control}
                    name="email"
                    label="Recovery Email"
                    hint="Used to reach you if you're locked out. You can also sign in with it — here and in mail or calendar apps — but it is not a mailbox address TinyCld hosts."
                    onBlur={saveIfValid}
                />
            </View>

            <ChangePassword />
        </View>
    )
}

function ChangePassword() {
    const [isOpen, setIsOpen] = useState(false)

    if (!isOpen) {
        return (
            <Pressable
                onPress={() => setIsOpen(true)}
                className="self-start rounded-lg px-3 py-2 border border-border"
            >
                <Text className="text-foreground font-semibold">Change password</Text>
            </Pressable>
        )
    }

    return <ChangePasswordForm onDone={() => setIsOpen(false)} />
}

function ChangePasswordForm({ onDone }: { onDone: () => void }) {
    const { user } = useAuth()
    const primaryFg = useThemeColor('primary-foreground')

    const {
        control,
        setError,
        getValues,
        handleSubmit,
        reset,
        formState: { errors, isSubmitted },
    } = useForm({
        resolver: zodResolver(passwordSchema),
        defaultValues: { oldPassword: '', password: '', passwordConfirm: '' },
    })

    const change = useMutation({
        mutationFn: (data: z.infer<typeof passwordSchema>) =>
            changeMyPassword({
                email: user.email,
                oldPassword: data.oldPassword,
                newPassword: data.password,
                passwordConfirm: data.passwordConfirm,
            }),
        onSuccess: () => {
            notify.emit({ event: 'account.password_changed', title: 'Password changed' })
            reset()
            onDone()
        },
        onError: handleMutationErrorsWithForm({
            setError,
            getValues,
            operation: 'change-password',
        }),
    })

    const handleCancel = () => {
        if (change.isPending) return
        reset()
        onDone()
    }

    const submit = handleSubmit(data => change.mutate(data))

    return (
        <SectionCard>
            <View className="gap-1">
                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                <TextInput
                    control={control}
                    name="oldPassword"
                    label="Current password"
                    secureTextEntry
                    autoComplete="current-password"
                    textContentType="password"
                />
                <TextInput
                    control={control}
                    name="password"
                    label="New password"
                    hint="At least 8 characters"
                    secureTextEntry
                    autoComplete="new-password"
                    textContentType="newPassword"
                />
                <TextInput
                    control={control}
                    name="passwordConfirm"
                    label="Confirm new password"
                    secureTextEntry
                    autoComplete="new-password"
                    textContentType="newPassword"
                />

                <View className="flex-row gap-3 mt-1">
                    <ServerActionButton onPress={submit} isDisabled={change.isPending}>
                        {change.isPending ? (
                            <ActivityIndicator size="small" color={primaryFg} />
                        ) : (
                            <ButtonText>Save</ButtonText>
                        )}
                    </ServerActionButton>
                    <Button variant="outline" onPress={handleCancel} isDisabled={change.isPending}>
                        <ButtonText>Cancel</ButtonText>
                    </Button>
                </View>
            </View>
        </SectionCard>
    )
}
