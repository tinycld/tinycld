import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { useReportHeight } from '@tinycld/core/lib/web-forms/parent-messages'
import { usePublicForm } from '@tinycld/core/lib/web-forms/use-public-form'
import { useLocalSearchParams } from 'expo-router'
import type { ReactNode } from 'react'
import { Text, View } from 'react-native'
import { ScopedTheme } from 'uniwind'

type ThemeParam = 'light' | 'dark' | 'auto'

function themeOf(raw: string | undefined): ThemeParam {
    return raw === 'light' || raw === 'dark' ? raw : 'auto'
}

function Themed({ theme, children }: { theme: ThemeParam; children: ReactNode }) {
    const body = <View className="bg-background p-4">{children}</View>
    if (theme === 'auto') return body
    return <ScopedTheme theme={theme}>{body}</ScopedTheme>
}

function Message({ text, testID }: { text: string; testID: string }) {
    return (
        <Text className="text-foreground text-base" testID={testID}>
            {text}
        </Text>
    )
}

export default function PublicFormScreen() {
    const params = useLocalSearchParams<{ token: string; theme?: string }>()
    const state = usePublicForm(params.token ?? '')
    useReportHeight()
    const theme = themeOf(params.theme)
    if (state.status === 'gone')
        return (
            <Themed theme={theme}>
                <Message text="This form is not available." testID="web-form-gone" />
            </Themed>
        )
    if (state.status === 'error')
        return (
            <Themed theme={theme}>
                <Message
                    text="The form could not load. Please reload the page."
                    testID="web-form-error"
                />
            </Themed>
        )
    if (state.status === 'loading') return <Themed theme={theme}>{null}</Themed>
    return (
        <Themed theme={theme}>
            <DocumentTitle title={state.form.name} includeOrg={false} />
            <Message text={state.form.name} testID="web-form-title" />
        </Themed>
    )
}
