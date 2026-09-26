import { SETUP_CONTINUE_TEST_ID } from '@tinycld/core/lib/setup/step-ids'
import { Button, ButtonText } from '@tinycld/core/ui/button'

interface SetupContinueButtonProps {
    onPress: () => void
    isDisabled?: boolean
}

/**
 * The button that finishes a setup step. Package steps use it too, so e2e
 * helpers can advance any step by its id instead of by its label.
 */
export function SetupContinueButton({ onPress, isDisabled }: SetupContinueButtonProps) {
    return (
        <Button
            className="self-start"
            onPress={onPress}
            isDisabled={isDisabled}
            testID={SETUP_CONTINUE_TEST_ID}
        >
            <ButtonText>Continue</ButtonText>
        </Button>
    )
}
