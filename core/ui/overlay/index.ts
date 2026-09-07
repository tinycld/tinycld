// The overlay engine. Internal to core/ui: packages use Dialog, Sheet,
// Popover and Menu, never these parts. See docs/overlays.md.
export { LayerEscape } from './escape'
export { useLayerFocus } from './focus'
export {
    OverlayHost,
    type OverlayHostName,
    OverlayPortal,
    OverlayProvider,
    useHasSheetHost,
} from './host'
export { type LayerRecord, layerToDismiss, useOverlayLayer } from './layer-stack'
