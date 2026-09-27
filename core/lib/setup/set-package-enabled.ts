/**
 * The status the client writes to turn a package on or off. Enabling always
 * writes `installed`: the server's pkg_registry hook corrects it to `bundled`
 * when the slug is compiled into this build (see pkg_enable_hook.go). The
 * client cannot decide that itself, because a disabled row no longer says
 * where it came from.
 */
export function enabledStatusFor(isEnabled: boolean): 'installed' | 'disabled' {
    return isEnabled ? 'installed' : 'disabled'
}
