import { useMutation } from '@tinycld/core/lib/mutations'
import { appHref } from '@tinycld/core/lib/org-routes'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useRouter } from 'expo-router'
import { Text, View } from 'react-native'

/** The last screen: marks the wizard complete and opens the workspace. */
export function DoneStep({ complete }: { complete: () => Promise<void> }) {
    const router = useRouter()
    const { org } = useOrgInfo()
    const name = org?.name ?? 'Your workspace'
    const open = useMutation({
        mutationFn: complete,
        onSuccess: () => router.replace(appHref('')),
    })
    return (
        <View className="max-w-[440px] gap-4">
            <Text className="text-2xl font-bold text-foreground">{`${name} is ready`}</Text>
            <Button
                className="self-start"
                onPress={() => open.mutate()}
                isDisabled={open.isPending}
            >
                <ButtonText>{`Open ${name}`}</ButtonText>
            </Button>
        </View>
    )
}
