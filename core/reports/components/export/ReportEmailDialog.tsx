import { useAuth } from '@tinycld/core/lib/auth'
import { handleMutationErrorsWithForm, isRecord } from '@tinycld/core/lib/errors'
import { log } from '@tinycld/core/lib/logger'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { DIALOG_SUBMIT_SHORTCUT, Dialog } from '@tinycld/core/ui/dialog'
import {
    ButtonGroup,
    type Control,
    type FieldErrors,
    FormErrorSummary,
    TextAreaInput,
    TextInput,
    useForm,
    useWatch,
    zodResolver,
} from '@tinycld/core/ui/form'
import { type RefObject, useRef } from 'react'
import { Text, type TextInputInstance, View } from 'react-native'
import type { ReportExportModel } from '../../hooks/useReportExport'
import { EXPORT_FORMATS, sendReportEmail } from '../../lib/export-client'
import {
    emailDefaults,
    parseAddresses,
    type ReportEmailValues,
    reportEmailSchema,
    senderNameOf,
    sentToastTitle,
} from '../../lib/export-email-form'

const TITLE = 'Email report'
const SEND = 'Send'
const SENDING = 'Sending…'

const FORMAT_OPTIONS = EXPORT_FORMATS.map(entry => ({ label: entry.label, value: entry.format }))

interface ReportEmailDialogProps {
    model: ReportExportModel
    heading: string
    periodWords: string
}

/**
 * The "Email report" dialog: a sheet on a phone. It is drawn only while open, so
 * each open starts from the defaults and no address is kept between sends.
 */
export function ReportEmailDialog({ model, heading, periodWords }: ReportEmailDialogProps) {
    if (!model.email.isOpen) return null
    return <OpenEmailDialog model={model} heading={heading} periodWords={periodWords} />
}

/** A refusal the reader can cause (a limit, a bad address) is expected; any other failure is a defect. */
function logRefusal(error: unknown) {
    const status = isRecord(error) && typeof error.status === 'number' ? error.status : 0
    if (status >= 400 && status < 500) {
        log.info('reports.email', 'report email refused', { status })
        return
    }
    log.error('reports.email', error)
}

/**
 * The server's refusal, at the top of the body so the reader sees it without
 * a scroll. Only the refusal shows here; field errors draw under their fields.
 */
function RefusalSummary({
    errors,
    isSubmitted,
}: {
    errors: FieldErrors<ReportEmailValues>
    isSubmitted: boolean
}) {
    const message = errors.root?.message
    if (!isSubmitted || !message) return null
    const serverErrors = { form: { type: 'server', message } }
    return (
        <View className="mb-3">
            <FormErrorSummary
                errors={serverErrors}
                isEnabled
                title={null}
                variant="destructive"
                testID="report-email-refusal"
            />
        </View>
    )
}

// Every hook runs here, outside the Dialog's content: on native the overlay host
// drops the consumer's contexts.
function OpenEmailDialog({ model, heading, periodWords }: ReportEmailDialogProps) {
    const { user } = useAuth()
    const toRef = useRef<TextInputInstance>(null)
    const { control, handleSubmit, setError, getValues, formState } = useForm<ReportEmailValues>({
        resolver: zodResolver(reportEmailSchema),
        defaultValues: emailDefaults({
            title: model.title,
            heading,
            periodWords,
            senderName: senderNameOf(user),
        }),
    })
    const format = useWatch({ control, name: 'format' })
    const showRefusal = handleMutationErrorsWithForm({
        setError,
        getValues,
        operation: 'reports.email',
        rootMessage: true,
    })
    const send = useMutation({
        mutationKey: ['reports', 'email'],
        // Offline is a refusal to show now, not a request to hold.
        networkMode: 'always',
        mutationFn: (values: ReportEmailValues) =>
            sendReportEmail(model.request ?? missingRequest(), values.format, {
                to: parseAddresses(values.to),
                cc: parseAddresses(values.cc),
                subject: values.subject,
                message: values.message,
            }),
        onSuccess: (_answer, values) => {
            model.email.onClose()
            useToastStore.getState().addToast({
                title: sentToastTitle(parseAddresses(values.to), parseAddresses(values.cc)),
                variant: 'success',
                duration: 4000,
            })
        },
        onError: error => {
            logRefusal(error)
            showRefusal(error)
        },
    })
    const isSending = send.isPending
    // A rerun that lands while the dialog is open (a change by someone else) disables
    // Send until the attachment line names the file the server will make again.
    const isSendDisabled = isSending || model.isDisabled
    const onSend = () => void handleSubmit(values => send.mutate(values))()
    const onClose = isSending ? () => {} : model.email.onClose
    return (
        <Dialog
            isOpen
            onClose={onClose}
            title={TITLE}
            size="lg"
            testID="report-email-dialog"
            initialFocusRef={toRef}
        >
            <Dialog.Body>
                <RefusalSummary errors={formState.errors} isSubmitted={formState.isSubmitted} />
                <EmailFields
                    control={control}
                    toRef={toRef}
                    isSending={isSending}
                    attachmentName={model.fileNameOf(format)}
                />
            </Dialog.Body>
            <Dialog.Footer>
                <Dialog.CancelButton
                    onPress={model.email.onClose}
                    isDisabled={isSending}
                    testID="report-email-cancel"
                />
                <Dialog.ActionButton
                    label={isSending ? SENDING : SEND}
                    onPress={onSend}
                    isDisabled={isSendDisabled}
                    shortcut={DIALOG_SUBMIT_SHORTCUT}
                    testID="report-email-send"
                />
            </Dialog.Footer>
        </Dialog>
    )
}

/** The send is offered only with a request (Export is disabled without one). */
function missingRequest(): never {
    throw new Error('The report has no export request.')
}

interface EmailFieldsProps {
    control: Control<ReportEmailValues>
    toRef: RefObject<TextInputInstance | null>
    isSending: boolean
    attachmentName: string
}

function EmailFields({ control, toRef, isSending, attachmentName }: EmailFieldsProps) {
    const isEditable = !isSending
    return (
        <>
            <TextInput
                control={control}
                name="to"
                label="To"
                testID="report-email-to"
                inputRef={toRef}
                hint="Separate addresses with commas, semicolons or spaces."
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="email-address"
                editable={isEditable}
            />
            <TextInput
                control={control}
                name="cc"
                label="Cc"
                testID="report-email-cc"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="email-address"
                editable={isEditable}
            />
            <TextInput
                control={control}
                name="subject"
                label="Subject"
                testID="report-email-subject"
                editable={isEditable}
            />
            <TextAreaInput
                control={control}
                name="message"
                label="Message"
                testID="report-email-message"
                numberOfLines={6}
                editable={isEditable}
            />
            <ButtonGroup
                control={control}
                name="format"
                label="Attach as"
                options={FORMAT_OPTIONS}
                horizontal
                disabled={isSending}
            />
            <Text testID="report-email-attachment" className="text-sm text-muted-foreground">
                {attachmentName}
            </Text>
        </>
    )
}
