import { deleteMyAccount } from '@tinycld/core/lib/account'
import { useAuth } from '@tinycld/core/lib/auth'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { CONNECT_HREF } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { Controller, isFormSubmittable, useForm, type z, zodResolver } from '@tinycld/core/ui/form'
import { router } from 'expo-router'
import { useMemo } from 'react'
import { ActivityIndicator, Text, TextInput, View } from 'react-native'
import { ContentPlanPicker, usePeers } from './account/OffboardDialog'
import { deleteAccountSchemaFor } from './account/offboard-schemas'

interface DeleteAccountModalProps {
    isVisible: boolean
    onClose: () => void
}

type DeleteAccountValues = z.infer<ReturnType<typeof deleteAccountSchemaFor>>

// useDeleteAccountForm owns the whole flow so the submit contract is testable:
// what the picker shows and what the mutation sends must be the same thing.
export function useDeleteAccountForm(onClose: () => void) {
    const { user, logout } = useAuth()
    // Who could inherit this user's files/documents/comments. Empty when
    // they're the only non-guest account, in which case the picker hides and
    // the plan stays undefined — content stays, attributed to them.
    const peers = usePeers(user.id)
    const hasPeers = peers.length > 0
    const schema = useMemo(
        () => deleteAccountSchemaFor(user.email, hasPeers),
        [user.email, hasPeers]
    )

    const form = useForm({
        resolver: zodResolver(schema),
        defaultValues: { confirm: '', plan: undefined },
        mode: 'onChange',
    })

    const mutation = useMutation({
        mutationFn: async (values: DeleteAccountValues) => {
            await deleteMyAccount(values.confirm, values.plan)
        },
        onSuccess: () => {
            logout()
            router.replace(CONNECT_HREF)
        },
        onError: err => form.setError('root', { type: 'server', message: errorToString(err) }),
    })

    const canSubmit = isFormSubmittable(form.formState) && !mutation.isPending

    const handleCancel = () => {
        if (mutation.isPending) return
        onClose()
    }

    const handleSubmit = () => void form.handleSubmit(values => mutation.mutate(values))()

    return {
        user,
        form,
        peers,
        isPending: mutation.isPending,
        canSubmit,
        handleCancel,
        handleSubmit,
    }
}

// Mounted only while visible, so each open starts from a blank confirmation and
// no plan rather than whatever was left from the last cancel.
export function DeleteAccountModal(props: DeleteAccountModalProps) {
    if (!props.isVisible) return null
    return <DeleteAccountModalOpen {...props} />
}

function DeleteAccountModalOpen({ onClose }: DeleteAccountModalProps) {
    const { user, form, peers, isPending, canSubmit, handleCancel, handleSubmit } =
        useDeleteAccountForm(onClose)
    const { control, formState } = form

    const mutedColor = useThemeColor('muted-foreground')
    const backdropColor = useThemeColor('overlay-backdrop')
    const dangerFg = useThemeColor('danger-foreground')

    return (
        <View
            className="absolute top-0 left-0 right-0 bottom-0 justify-center items-center"
            style={{ zIndex: 200, backgroundColor: backdropColor }}
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
                <Text className="text-[22px] font-bold mb-1 text-foreground">Delete account</Text>
                <Text className="text-sm mb-2 text-muted-foreground">
                    This action is permanent and cannot be undone. Your name, email and avatar are
                    removed and you're signed out everywhere. Disable your account instead if you
                    might come back.
                </Text>
                <Text className="text-sm mb-6 text-muted-foreground">
                    Signed in as <Text className="font-semibold text-foreground">{user.email}</Text>
                </Text>

                <View className="mb-6">
                    <Controller
                        control={control}
                        name="plan"
                        render={({ field, fieldState }) => (
                            <>
                                <ContentPlanPicker
                                    peers={peers}
                                    subjectLabel="your"
                                    value={field.value}
                                    onChange={field.onChange}
                                    disabled={isPending}
                                />
                                <FieldError message={fieldState.error?.message} />
                            </>
                        )}
                    />
                </View>

                <ServerError message={formState.errors.root?.message} />

                <View className="mb-6">
                    <Text className="mb-1.5 text-sm font-semibold text-foreground">
                        Type your email to confirm
                    </Text>
                    <Controller
                        control={control}
                        name="confirm"
                        render={({ field }) => (
                            <TextInput
                                className="border border-border rounded-lg p-3 text-base text-foreground bg-surface-secondary"
                                value={field.value}
                                onChangeText={field.onChange}
                                onBlur={field.onBlur}
                                placeholder={user.email}
                                placeholderTextColor={mutedColor}
                                keyboardType="email-address"
                                autoCapitalize="none"
                                autoComplete="email"
                                editable={!isPending}
                                onSubmitEditing={handleSubmit}
                            />
                        )}
                    />
                </View>

                <ServerActionButton
                    variant="destructive"
                    className="mb-3 w-full"
                    onPress={handleSubmit}
                    isDisabled={!canSubmit}
                >
                    {isPending ? (
                        <ActivityIndicator color={dangerFg} size="small" />
                    ) : (
                        <ButtonText>Delete account</ButtonText>
                    )}
                </ServerActionButton>

                <Button
                    variant="outline"
                    className="w-full"
                    onPress={handleCancel}
                    isDisabled={isPending}
                >
                    <ButtonText>Cancel</ButtonText>
                </Button>
            </View>
        </View>
    )
}

function FieldError({ message }: { message: string | undefined }) {
    if (!message) return null
    return <Text className="mt-1.5 text-xs text-danger">{message}</Text>
}

function ServerError({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <View className="rounded-lg p-3 mb-4 bg-danger-soft">
            <Text className="text-sm text-danger">{message}</Text>
        </View>
    )
}
