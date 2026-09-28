// The overlay engine. Internal to core/ui: packages use Dialog, Sheet,
// Popover and Menu, never these parts. See docs/overlays.md.
//
// The inert machinery is deliberately NOT re-exported here. `inert-siblings.ts`
// is a leaf — it imports nothing — and its consumers reach it directly
// (`ui/overlay/inert-siblings`, `ui/overlay/use-inert-exempt`). Going through
// this barrel would drag `host.tsx` and the whole engine into the graph of
// every module that only wants to stay interactive, which is how a plain leaf
// dependency turns into an import cycle the bundler resolves differently from
// the test runner.
export { LayerEscape } from './escape'
export { useLayerFocus } from './focus'
export {
    OverlayHost,
    type OverlayHostName,
    OverlayPortal,
    OverlayProvider,
    useHasSheetHost,
} from './host'
export {
    type LayerRecord,
    layerToDismiss,
    modalLayerCount,
    useIsModalLayerOpen,
    useOverlayLayer,
    wasConsumedByLayerDismissal,
} from './layer-stack'
