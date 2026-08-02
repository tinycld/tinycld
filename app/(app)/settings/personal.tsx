import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { SortableDragHandle, SortableList } from '@tinycld/core/components/SortableList'
import { AboutSection } from '@tinycld/core/components/settings/AboutSection'
import { DisableAccountSection } from '@tinycld/core/components/settings/account/DisableAccountSection'
import { DeleteAccountSection } from '@tinycld/core/components/settings/DeleteAccountSection'
import { DisconnectServerSection } from '@tinycld/core/components/settings/DisconnectServerSection'
import { ServersSection } from '@tinycld/core/components/settings/ServersSection'
import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { changeMyPassword } from '@tinycld/core/lib/account-password'
import { useAuth } from '@tinycld/core/lib/auth'
import { COLOR_THEMES, type ColorThemeSlug } from '@tinycld/core/lib/color-themes'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import type { PackageManifest } from '@tinycld/core/lib/packages/types'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useAccessiblePackages } from '@tinycld/core/lib/use-accessible-packages'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useColorTheme } from '@tinycld/core/lib/use-color-theme'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import {
    type MailNotifyMode,
    type NotificationPreferences,
    useNotificationPreferences,
} from '@tinycld/core/lib/use-notification-preferences'
import { usePushSubscription } from '@tinycld/core/lib/use-push-subscription'
import { type ThemePreference, useThemePreference } from '@tinycld/core/lib/use-theme-preference'
import { useUserPreference } from '@tinycld/core/lib/use-user-preference'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Switch } from '@tinycld/core/ui/switch'
import { ArrowLeft, Check, RotateCcw } from 'lucide-react-native'
import { useCallback, useMemo, useState } from 'react'
import {
    ActivityIndicator,
    Platform,
    Pressable,
    ScrollView,
    StyleSheet,
    Text,
    View,
} from 'react-native'
import { GestureHandlerRootView } from 'react-native-gesture-handler'

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

export default function PersonalSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const foregroundColor = useThemeColor('foreground')

    return (
        <GestureHandlerRootView className="flex-1">
            <DocumentTitle pkg="Settings" title="Personal" />
            <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
                <View className="p-5 max-w-[600px] gap-6">
                    <View className="flex-row gap-3 items-center">
                        <Pressable onPress={navigateBack}>
                            <ArrowLeft size={24} color={foregroundColor} />
                        </Pressable>
                        <Text className="text-foreground text-[22px] font-bold">
                            Personal Settings
                        </Text>
                    </View>

                    <ProfileSection />
                    <AppearanceSection />
                    <NotificationsSection />
                    <NavigationSection />
                    <ServersSection />
                    <DisconnectServerSection />
                    <AboutSection />
                    <DisableAccountSection />
                    <DeleteAccountSection />
                </View>
            </ScrollView>
        </GestureHandlerRootView>
    )
}

function ProfileSection() {
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
            <Text className="text-foreground text-xl font-bold">Profile</Text>

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
                    <Pressable
                        onPress={submit}
                        disabled={change.isPending}
                        className={`rounded-lg px-4 py-2.5 bg-primary ${change.isPending ? 'opacity-50' : 'opacity-100'}`}
                    >
                        {change.isPending ? (
                            <ActivityIndicator size="small" color={primaryFg} />
                        ) : (
                            <Text className="text-primary-foreground font-semibold">Save</Text>
                        )}
                    </Pressable>
                    <Pressable
                        onPress={handleCancel}
                        disabled={change.isPending}
                        className="rounded-lg px-4 py-2.5 border border-border"
                    >
                        <Text className="text-foreground font-semibold">Cancel</Text>
                    </Pressable>
                </View>
            </View>
        </SectionCard>
    )
}

const THEME_OPTIONS: { value: ThemePreference; label: string; description: string }[] = [
    { value: 'system', label: 'System', description: 'Follow your device settings' },
    { value: 'light', label: 'Light', description: 'Always use light theme' },
    { value: 'dark', label: 'Dark', description: 'Always use dark theme' },
]

function AppearanceSection() {
    const primaryColor = useThemeColor('primary')
    const { preference, setPreference, resolved } = useThemePreference()
    const { colorTheme, setColorTheme } = useColorTheme()

    return (
        <View className="gap-3">
            <Text className="text-foreground text-xl font-bold">Appearance</Text>
            <SectionCard>
                <View className="gap-4">
                    <View className="gap-1">
                        {THEME_OPTIONS.map(option => (
                            <Pressable
                                key={option.value}
                                onPress={() => setPreference(option.value)}
                                className="flex-row items-center py-2.5 px-1 rounded-lg"
                            >
                                <View className="flex-1">
                                    <Text className="text-foreground text-base font-semibold">
                                        {option.label}
                                    </Text>
                                    <Text className="text-muted-foreground text-[13px]">
                                        {option.description}
                                    </Text>
                                </View>
                                {preference === option.value && (
                                    <Check size={20} color={primaryColor} />
                                )}
                            </Pressable>
                        ))}
                    </View>

                    <View className="h-px bg-muted-foreground/30" />

                    <View className="gap-2">
                        <Text className="text-foreground text-sm font-semibold">Accent Color</Text>
                        <ColorThemePicker
                            selected={colorTheme}
                            onSelect={setColorTheme}
                            isDark={resolved === 'dark'}
                        />
                    </View>
                </View>
            </SectionCard>
        </View>
    )
}

function ColorThemePicker({
    selected,
    onSelect,
    isDark,
}: {
    selected: ColorThemeSlug
    onSelect: (slug: ColorThemeSlug) => void
    isDark: boolean
}) {
    const borderColor = useThemeColor('border')
    const onSwatchColor = useThemeColor('primary-foreground')

    return (
        <View className="flex-row gap-4 flex-wrap">
            {COLOR_THEMES.map(theme => {
                const isActive = selected === theme.slug
                const swatchColor = isDark ? theme.swatchDark : theme.swatch
                return (
                    <Pressable
                        key={theme.slug}
                        onPress={() => onSelect(theme.slug)}
                        className="items-center gap-1.5"
                    >
                        <View
                            className="items-center justify-center"
                            style={{
                                width: 40,
                                height: 40,
                                borderRadius: 20,
                                backgroundColor: swatchColor,
                                borderWidth: isActive ? 3 : 1,
                                borderColor: isActive ? swatchColor : borderColor,
                            }}
                        >
                            {isActive && <Check size={18} color={onSwatchColor} />}
                        </View>
                        <Text
                            className={`text-[11px] ${isActive ? 'font-semibold text-foreground' : 'font-normal text-muted-foreground'}`}
                        >
                            {theme.label}
                        </Text>
                    </Pressable>
                )
            })}
        </View>
    )
}

function NotificationsSection() {
    const { isSupported, isSubscribed, subscribe, unsubscribe, isPending } = usePushSubscription()

    const handlePushToggle = () => {
        if (isSubscribed) {
            unsubscribe()
        } else {
            subscribe()
        }
    }

    return (
        <View className="gap-3">
            <Text className="text-foreground text-xl font-bold">Notifications</Text>
            <PushToggle
                isSupported={Platform.OS === 'web' && isSupported}
                isNative={Platform.OS !== 'web'}
                isSubscribed={isSubscribed}
                isPending={isPending}
                onToggle={handlePushToggle}
            />
            <NotificationTypeToggles />
        </View>
    )
}

function PushToggle({
    isSupported,
    isNative,
    isSubscribed,
    isPending,
    onToggle,
}: {
    isSupported: boolean
    isNative: boolean
    isSubscribed: boolean
    isPending: boolean
    onToggle: () => void
}) {
    if (isNative) {
        return (
            <SectionCard>
                <Text className="text-foreground text-base">
                    Push notifications are managed by your device settings.
                </Text>
            </SectionCard>
        )
    }

    if (!isSupported) {
        return (
            <SectionCard>
                <Text className="text-muted-foreground text-[13px]">
                    Your browser does not support push notifications.
                </Text>
            </SectionCard>
        )
    }

    return (
        <SectionCard>
            <Pressable onPress={onToggle} disabled={isPending}>
                <View className="flex-row items-center gap-3">
                    <View className="flex-1 gap-0.5">
                        <Text className="text-foreground text-base font-semibold">
                            Browser Push Notifications
                        </Text>
                        <Text className="text-muted-foreground text-[13px]">
                            Receive calendar reminders even when the browser tab is closed.
                        </Text>
                    </View>
                    {isPending ? (
                        <ActivityIndicator size="small" />
                    ) : (
                        <Switch value={isSubscribed} onValueChange={onToggle} />
                    )}
                </View>
            </Pressable>
        </SectionCard>
    )
}

const NOTIF_GROUPS: {
    label: string
    types: { key: keyof NotificationPreferences; label: string }[]
}[] = [
    {
        label: 'Calendar',
        types: [
            { key: 'calendar_reminder', label: 'Event reminders' },
            { key: 'calendar_invite', label: 'Calendar invites' },
            { key: 'calendar_subscription_error', label: 'Subscription sync errors' },
        ],
    },
    {
        label: 'Mail',
        types: [{ key: 'mail_new_message', label: 'New messages' }],
    },
    {
        label: 'Drive',
        types: [{ key: 'drive_file_shared', label: 'Files shared with you' }],
    },
    {
        label: 'General',
        types: [
            { key: 'org_invite', label: 'Organization invites' },
            { key: 'system_error', label: 'System errors' },
        ],
    },
]

const MAIL_MODE_OPTIONS: { value: MailNotifyMode; label: string; description: string }[] = [
    {
        value: 'batched',
        label: 'All messages (batched)',
        description: 'Notify for all incoming messages, batched every 2 minutes',
    },
    {
        value: 'important_only',
        label: 'Important only',
        description: 'Only notify for messages from your contacts',
    },
]

function NotificationTypeToggles() {
    const { prefs, setTypeEnabled, mailMode, setMailMode } = useNotificationPreferences()

    return (
        <SectionCard>
            <View className="gap-4">
                {NOTIF_GROUPS.map(group => (
                    <View key={group.label} className="gap-1.5">
                        <Text
                            className="text-muted-foreground text-[13px] font-semibold uppercase"
                            style={{ letterSpacing: 0.5 }}
                        >
                            {group.label}
                        </Text>
                        {group.types.map(type => (
                            <NotifTypeRow
                                key={type.key}
                                label={type.label}
                                enabled={prefs[type.key]}
                                onToggle={val => setTypeEnabled(type.key, val)}
                            />
                        ))}
                        <MailModeSelector
                            isVisible={group.label === 'Mail' && prefs.mail_new_message}
                            mailMode={mailMode}
                            onSelect={setMailMode}
                        />
                    </View>
                ))}
            </View>
        </SectionCard>
    )
}

function NotifTypeRow({
    label,
    enabled,
    onToggle,
}: {
    label: string
    enabled: boolean
    onToggle: (val: boolean) => void
}) {
    return (
        <View className="flex-row items-center justify-between py-1.5">
            <Text className="text-foreground text-[15px]">{label}</Text>
            <Switch value={enabled} onValueChange={onToggle} />
        </View>
    )
}

function MailModeSelector({
    isVisible,
    mailMode,
    onSelect,
}: {
    isVisible: boolean
    mailMode: MailNotifyMode
    onSelect: (mode: MailNotifyMode) => void
}) {
    const primaryColor = useThemeColor('primary')

    if (!isVisible) return null

    return (
        <View className="gap-1 ml-2">
            {MAIL_MODE_OPTIONS.map(opt => (
                <Pressable
                    key={opt.value}
                    onPress={() => onSelect(opt.value)}
                    className="flex-row items-center py-1.5 gap-2"
                >
                    <View
                        className={`w-4 h-4 rounded-full border-2 items-center justify-center ${mailMode === opt.value ? 'border-primary' : 'border-muted-foreground'}`}
                    >
                        <RadioDot isVisible={mailMode === opt.value} color={primaryColor} />
                    </View>
                    <View>
                        <Text className="text-foreground text-sm">{opt.label}</Text>
                        <Text className="text-muted-foreground text-xs">{opt.description}</Text>
                    </View>
                </Pressable>
            ))}
        </View>
    )
}

function RadioDot({ isVisible, color }: { isVisible: boolean; color: string }) {
    if (!isVisible) return null
    return (
        <View
            style={{
                width: 8,
                height: 8,
                borderRadius: 4,
                backgroundColor: color,
            }}
        />
    )
}

function SectionCard({ children }: { children: React.ReactNode }) {
    return (
        <View className="rounded-xl border p-4 bg-surface-secondary border-border">{children}</View>
    )
}

function deriveOrder(packages: PackageManifest[], savedOrder: string[]): string[] {
    if (!savedOrder.length) {
        return [...packages]
            .sort((a, b) => (a.nav?.order ?? 99) - (b.nav?.order ?? 99))
            .map(a => a.slug)
    }
    const pkgSlugs = new Set(packages.map(a => a.slug))
    const ordered = savedOrder.filter(slug => pkgSlugs.has(slug))
    const missing = [...packages]
        .filter(a => !savedOrder.includes(a.slug))
        .sort((a, b) => (a.nav?.order ?? 99) - (b.nav?.order ?? 99))
        .map(a => a.slug)
    return [...ordered, ...missing]
}

function NavigationSection() {
    const foregroundColor = useThemeColor('foreground')
    const surfaceBg = useThemeColor('surface-secondary')
    const packages = useAccessiblePackages()
    const [savedOrder, setSavedOrder] = useUserPreference('core', 'pkg_order', [] as string[])
    const localOrder = useMemo(() => deriveOrder(packages, savedOrder), [packages, savedOrder])

    const pkgMap = new Map(packages.map(a => [a.slug, a]))
    const isCustomized = savedOrder.length > 0

    const handleDragEnd = useCallback(
        (data: string[]) => {
            setSavedOrder(data)
        },
        [setSavedOrder]
    )

    function resetOrder() {
        setSavedOrder([] as string[])
    }

    function renderItem({ item }: { item: string; index: number }) {
        const pkg = pkgMap.get(item)
        if (!pkg) return null
        const Icon = getIcon(pkg.nav?.icon ?? '')

        return (
            <View
                className="flex-row items-center justify-between px-4 py-3.5 border-border"
                style={{
                    borderBottomWidth: StyleSheet.hairlineWidth,
                    backgroundColor: surfaceBg,
                }}
            >
                <View className="flex-row items-center gap-3">
                    <SortableDragHandle />
                    <Icon size={20} color={foregroundColor} />
                    <Text className="text-base text-foreground">{pkg.nav?.label}</Text>
                </View>
            </View>
        )
    }

    const keyExtractor = useCallback((slug: string) => slug, [])

    return (
        <View className="gap-3">
            <Text className="text-xl font-bold text-foreground">Navigation</Text>

            <Text className="text-[13px] text-muted-foreground">
                Drag to reorder your apps. The order is reflected in the sidebar and mobile tab bar.
            </Text>

            <View className="rounded-xl border border-border overflow-hidden">
                <SortableList
                    data={localOrder}
                    keyExtractor={keyExtractor}
                    onReorder={handleDragEnd}
                    renderItem={renderItem}
                />
            </View>

            <ResetButton isVisible={isCustomized} onPress={resetOrder} />
        </View>
    )
}

function ResetButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    const mutedColor = useThemeColor('muted-foreground')

    if (!isVisible) return null

    return (
        <Pressable
            onPress={onPress}
            className="flex-row items-center gap-1.5 px-3 py-2 rounded-lg border border-border self-start"
        >
            <RotateCcw size={14} color={mutedColor} />
            <Text className="text-foreground">Reset to Default</Text>
        </Pressable>
    )
}
