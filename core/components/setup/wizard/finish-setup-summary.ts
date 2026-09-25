/** "<doneCount> of <total> done" — the Finish setup card's subtitle. */
export function finishSetupSubtitleOf(summary: { doneCount: number; total: number }): string {
    return `${summary.doneCount} of ${summary.total} done`
}
