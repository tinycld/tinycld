/**
 * Whether a submitted value differs from what the session opened with.
 *
 * An unchanged edit is a cancel rather than a write — EditableText's rule. The
 * baseline is snapshotted at acquire, so a realtime update arriving mid-edit
 * cannot become the comparison target.
 */
export function isNoOpEdit(next: string, baseline: string): boolean {
    return next.trim() === baseline.trim()
}
