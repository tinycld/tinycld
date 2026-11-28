export interface MemberPeers {
    name: string
    peerDependencies?: Record<string, string>
}

export interface UncoveredPeer {
    member: string
    peer: string
}

// Members declare framework and library deps as peers only; with
// `nodeLinker: hoisted` a peer installs only when something in the tree
// declares it as a real dependency. The app shell's own dependencies and the
// core version pins are the two places that do. A peer that neither names
// resolves only when some other package happens to pull it in.
export function findUncoveredPeers(
    members: MemberPeers[],
    shellDeps: ReadonlySet<string>,
    pinned: ReadonlySet<string>
): UncoveredPeer[] {
    const uncovered: UncoveredPeer[] = []
    for (const member of members) {
        for (const peer of Object.keys(member.peerDependencies ?? {})) {
            if (peer.startsWith('@tinycld/')) continue
            if (shellDeps.has(peer) || pinned.has(peer)) continue
            uncovered.push({ member: member.name, peer })
        }
    }
    return uncovered
}
