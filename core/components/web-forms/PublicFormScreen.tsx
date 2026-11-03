import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { MarkdownRenderer } from '@tinycld/core/components/help/MarkdownRenderer'
import { toDateString } from '@tinycld/core/lib/dates'
import { WEB_FORM_HOST_KEY } from '@tinycld/core/lib/forms/core-hosts'
import {
    applyDefaults,
    type EvalContext,
    evaluate,
    type FormValues,
} from '@tinycld/core/lib/forms/logic'
import { useFormHost } from '@tinycld/core/lib/forms/use-form-hosts'
import { buildSchema } from '@tinycld/core/lib/forms/values'
import { fileLimitIssues } from '@tinycld/core/lib/web-forms/file-limits'
import { reportHeight } from '@tinycld/core/lib/web-forms/parent-messages'
import { usePendingFiles } from '@tinycld/core/lib/web-forms/pending-files-store'
import { browserCanSolvePow } from '@tinycld/core/lib/web-forms/pow-capability'
import { type PublicForm, usePublicForm } from '@tinycld/core/lib/web-forms/use-public-form'
import { usePublicSubmit } from '@tinycld/core/lib/web-forms/use-public-submit'
import { zodResolver } from '@tinycld/core/ui/form'
import { FormRenderer, useSchemaRefResolver } from '@tinycld/core/ui/forms'
import { useLocalSearchParams } from 'expo-router'
import type { ReactNode } from 'react'
import { type Resolver, useForm } from 'react-hook-form'
import { type LayoutChangeEvent, ScrollView, Text, TextInput, View } from 'react-native'
import { ScopedTheme } from 'uniwind'

type ThemeParam = 'light' | 'dark' | 'auto'

function themeOf(raw: string | undefined): ThemeParam {
    return raw === 'light' || raw === 'dark' ? raw : 'auto'
}

const onContentLayout = (e: LayoutChangeEvent) => reportHeight(e.nativeEvent.layout.height)

// Scrolls on its own when opened directly; inside an iframe the embedding
// page sizes the frame to the reported content height.
function Themed({ theme, children }: { theme: ThemeParam; children: ReactNode }) {
    const body = (
        <ScrollView className="flex-1 bg-background">
            <View className="p-4" onLayout={onContentLayout}>
                {children}
            </View>
        </ScrollView>
    )
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

// The renderer's schema plus the upload limits, checked before anything is
// sent: the pending files live outside the form values, so the zod refine
// reads them from their store.
function useWebFormResolver(
    form: PublicForm,
    schemaRef: ReturnType<typeof useSchemaRefResolver>['schemaRef']
) {
    const resolver: Resolver<FormValues> = (values, context, options) => {
        const schema = schemaRef.current.superRefine((v, ctx) => {
            const issues = fileLimitIssues(
                form.definition,
                v,
                usePendingFiles.getState().files,
                form.maxUploadMb
            )
            for (const issue of issues) ctx.addIssue({ code: 'custom', ...issue })
        })
        return zodResolver(schema)(values, context, options)
    }
    return resolver
}

function usePublicFormBody(token: string, form: PublicForm, receivedAt: number) {
    const host = useFormHost(WEB_FORM_HOST_KEY)
    if (!host) throw new Error('core:web_form host missing from the registry')
    const context: EvalContext = { today: toDateString(new Date()), userId: '' }
    const defaults = applyDefaults(form.definition, context)
    const initialSchema = buildSchema(
        form.definition,
        host,
        evaluate(form.definition, defaults, context)
    )
    const { schemaRef } = useSchemaRefResolver(initialSchema)
    const resolver = useWebFormResolver(form, schemaRef)
    const rhf = useForm({ resolver, defaultValues: defaults })
    const submit = usePublicSubmit({ token, form, receivedAt, host, rhf })
    return {
        host,
        context,
        schemaRef,
        rhf,
        submit,
        submitLabel: submit.isSubmitting ? 'Sending…' : 'Send',
    }
}

// Off-screen and hidden from assistive tech; people never fill it, bots do.
function Honeypot({ onChangeText }: { onChangeText: (v: string) => void }) {
    return (
        <View
            style={{ position: 'absolute', left: -10000, width: 1, height: 1, overflow: 'hidden' }}
            aria-hidden
            importantForAccessibility="no-hide-descendants"
        >
            <TextInput
                onChangeText={onChangeText}
                autoComplete="off"
                tabIndex={-1}
                testID="web-form-hp"
            />
        </View>
    )
}

function Confirmation({ body }: { body: string }) {
    return (
        <View testID="web-form-confirmation">
            <MarkdownRenderer body={body} purpose="documentation" />
        </View>
    )
}

function PublicFormBody(p: { token: string; form: PublicForm; receivedAt: number }) {
    const b = usePublicFormBody(p.token, p.form, p.receivedAt)
    if (b.submit.isDone) return <Confirmation body={p.form.confirmation} />
    return (
        <View>
            <Honeypot onChangeText={b.submit.honeypot.onChangeText} />
            <FormRenderer
                host={b.host}
                ui={null}
                definition={p.form.definition}
                layout="wizard"
                form={b.rhf}
                context={b.context}
                schemaRef={b.schemaRef}
                pendingFiles
                submitLabel={b.submitLabel}
                onSubmit={b.submit.submit}
                testID="web-form"
            />
        </View>
    )
}

export default function PublicFormScreen() {
    const params = useLocalSearchParams<{ token: string; theme?: string }>()
    const token = params.token ?? ''
    const state = usePublicForm(token)
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
    const isInsecure = state.form.requirePow && !browserCanSolvePow()
    if (isInsecure)
        return (
            <Themed theme={theme}>
                <Message
                    text="This form needs a secure (https) connection."
                    testID="web-form-insecure"
                />
            </Themed>
        )
    return (
        <Themed theme={theme}>
            <DocumentTitle title={state.form.name} includeOrg={false} />
            <PublicFormBody token={token} form={state.form} receivedAt={state.receivedAt} />
        </Themed>
    )
}
