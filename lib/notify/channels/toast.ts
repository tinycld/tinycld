import type { DispatchInput, NotifyChannel } from '@tinycld/core/lib/notify/channels/types'
import { captureMessageToSentry } from '@tinycld/core/lib/sentry'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'

export const toastChannel: NotifyChannel = {
    name: 'toast',
    dispatch(input: DispatchInput) {
        captureMessageToSentry('toast-channel', 'dispatch', {
            event: input.event,
            title: input.title,
            variant: input.variant,
            ts: Date.now(),
        })
        useToastStore.getState().addToast({
            title: input.title,
            body: input.body,
            variant: input.variant,
            duration: input.durationMs ?? 4000,
        })
    },
}
