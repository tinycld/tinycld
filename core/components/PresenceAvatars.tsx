import { AvatarStack } from '@tinycld/core/components/AvatarStack'
import { useRemoteAwareness } from '@tinycld/core/lib/realtime/use-remote-awareness'
import { useMemo } from 'react'
import type { Awareness } from 'y-protocols/awareness'

interface PresenceUser {
    id: string
    name: string
    color: string
}

interface ParsedAwareness {
    user: PresenceUser
}

interface PresenceAvatarsProps {
    awareness: Awareness | null
    // Maximum number of avatar circles to render. Anything past the
    // limit collapses into a "+N" badge.
    max?: number
    size?: number
}

// PresenceAvatars renders a stacked row of initials representing the
// other users currently connected to the same realtime room. Generic
// over room kind — any awareness slot whose `user` field has
// {id, name, color} will appear.
//
// Server-published slots (text's saveStatus, future kinds) come through
// a separate channel (MsgServerSlot via onServerSlot) and never appear
// in y-protocols/awareness, so no special filtering is needed here. We
// still narrow on the parsed shape so a malformed slot (e.g. a peer
// using a different schema in the same room) is dropped cleanly.
export function PresenceAvatars({ awareness, max = 4, size = 24 }: PresenceAvatarsProps) {
    const options = useMemo(() => ({ parse: parseSlot, equals: sameUser }), [])
    const peers = useRemoteAwareness<ParsedAwareness>(awareness, options)

    if (peers.length === 0) return null

    return (
        <AvatarStack
            items={peers.map(peer => ({
                key: String(peer.clientID),
                name: peer.state.user.name,
                color: peer.state.user.color,
                colorKey: peer.state.user.id,
            }))}
            max={max}
            size={size}
            ring="background"
        />
    )
}

function parseSlot(raw: unknown): ParsedAwareness | null {
    if (raw == null || typeof raw !== 'object') return null
    const obj = raw as Record<string, unknown>
    const userObj = obj.user as Record<string, unknown> | undefined
    if (
        userObj == null ||
        typeof userObj.id !== 'string' ||
        typeof userObj.name !== 'string' ||
        typeof userObj.color !== 'string'
    ) {
        return null
    }
    return { user: { id: userObj.id, name: userObj.name, color: userObj.color } }
}

function sameUser(a: ParsedAwareness, b: ParsedAwareness): boolean {
    return a.user.id === b.user.id && a.user.name === b.user.name && a.user.color === b.user.color
}
