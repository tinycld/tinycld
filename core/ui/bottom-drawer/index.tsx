// Kept as a name only: the bottom sheet is `Sheet` in core/ui/sheet, which
// took this component's gesture, animation and host contract and added the
// title/body/footer structure the other surfaces share. Consumers still on
// this import move to `Sheet` as their package migrates (docs/plans/overlay-engine.md).
export { Sheet as BottomDrawer } from '@tinycld/core/ui/sheet'
