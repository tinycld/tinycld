import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { usePublication } from '@tinycld/core/lib/web-forms/use-publication'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { FormErrorSummary, NumberInput, TextInput, Toggle } from '@tinycld/core/ui/form'
import { useState } from 'react'
import { Text, View } from 'react-native'
import { DestinationsSection } from './DestinationsSection'

interface EmbedCodeProps {
    code: string | null
    isCopied: boolean
    onCopy: () => void
    onRegenerate: () => void
}

function EmbedCode({ code, isCopied, onCopy, onRegenerate }: EmbedCodeProps) {
    if (!code) return <Text className="text-muted-foreground">Save to get the embed code.</Text>
    return (
        <View className="gap-2">
            <Text className="text-foreground font-medium">Embed code</Text>
            <Text
                selectable
                className="text-foreground bg-muted rounded-md p-2 font-mono text-xs"
                testID="web-form-embed-code"
            >
                {code}
            </Text>
            <View className="flex-row flex-wrap gap-2">
                <Button size="sm" onPress={onCopy} testID="web-form-copy">
                    <ButtonText>{isCopied ? 'Copied' : 'Copy embed code'}</ButtonText>
                </Button>
                <Button
                    size="sm"
                    variant="outline"
                    onPress={onRegenerate}
                    testID="web-form-regenerate"
                >
                    <ButtonText>Regenerate link</ButtonText>
                </Button>
            </View>
        </View>
    )
}

function usePublishTab(formId: string, formName: string) {
    const p = usePublication(formId, formName)
    // Whether the regenerate confirm is open: local UI state.
    const [isConfirming, setIsConfirming] = useState(false)
    return {
        p,
        isConfirming,
        askRegenerate: () => setIsConfirming(true),
        cancelRegenerate: () => setIsConfirming(false),
        confirmRegenerate: () => {
            setIsConfirming(false)
            p.regenerate()
        },
    }
}

interface PublishTabProps {
    formId: string
    formName: string
    isVisible: boolean
}

// Mounted only while the tab shows, so the publication query runs only then.
export function PublishTab({ isVisible, ...rest }: PublishTabProps) {
    if (!isVisible) return null
    return <PublishPanel {...rest} />
}

function PublishPanel({ formId, formName }: Omit<PublishTabProps, 'isVisible'>) {
    const t = usePublishTab(formId, formName)
    const { control, formState } = t.p.form
    const powHint = t.p.powAvailable
        ? "The visitor's browser does about one second of work before it sends. Stops bulk spam."
        : 'Needs this server on https.'
    return (
        <View className="gap-4 max-w-2xl" testID="web-form-publish">
            <View className="flex-row items-center gap-2">
                <Text className="text-foreground text-lg font-semibold">Publish</Text>
                <HelpIcon topic="core:web-forms" />
            </View>
            <FormErrorSummary errors={formState.errors} isEnabled={formState.isSubmitted} />
            <Toggle
                control={control}
                name="enabled"
                label="Accept submissions"
                testID="web-form-enabled"
            />
            <TextInput
                control={control}
                name="allowedDomains"
                label="Websites that may show this form"
                hint="Space separated, e.g. https://example.com. Leave empty to allow any website."
                autoCapitalize="none"
                testID="web-form-domains"
            />
            <Toggle
                control={control}
                name="requirePow"
                label="Ask the browser to prove it is not a bot"
                hint={powHint}
                disabled={!t.p.powAvailable}
                testID="web-form-pow"
            />
            <TextInput
                control={control}
                name="confirmation"
                label="Message after sending"
                multiline
                testID="web-form-confirmation-input"
            />
            <NumberInput
                control={control}
                name="retentionDays"
                label="Keep submissions for (days)"
                min={1}
                max={3650}
                testID="web-form-retention"
            />
            <NumberInput
                control={control}
                name="maxUploadMb"
                label="Largest upload for each submission (MB)"
                min={1}
                max={100}
                testID="web-form-max-upload"
            />
            <Button
                onPress={t.p.save}
                isDisabled={t.p.isSaving || !t.p.canSave}
                className="self-start"
                testID="web-form-save"
            >
                <ButtonText>Save</ButtonText>
            </Button>
            <EmbedCode
                code={t.p.embed}
                isCopied={t.p.isCopied}
                onCopy={t.p.copy}
                onRegenerate={t.askRegenerate}
            />
            <DestinationsSection formId={formId} formName={formName} />
            <ConfirmDialog
                isOpen={t.isConfirming}
                title="Regenerate the link?"
                message="Every page that embeds the current code stops showing the form. You must paste the new code on those pages."
                confirmLabel="Regenerate"
                onConfirm={t.confirmRegenerate}
                onClose={t.cancelRegenerate}
            />
        </View>
    )
}
