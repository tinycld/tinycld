import { useMemo, useRef, useState } from 'react'
import { Animated, type LayoutChangeEvent } from 'react-native'

/**
 * D9: on a phone the report's top area (header, chips, notices, document header) is
 * the list's header and scrolls away with the rows. The column heading row floats
 * over the list: it sits in its slot under the top area (FlashList keeps an empty
 * slot of stickyHeaderConfig.offset after the list header), rises with it, and stops
 * at the top once the top area has gone. Sticky group copies stick at the offset,
 * right under it. One JS-driven scroll value moves the heading, and the phone's
 * sticky copies use the JS driver too, so both move together on web and native.
 * Heights are container layouts, not text measures.
 */
export function useScrollAwayTop() {
    const scrollY = useRef(new Animated.Value(0)).current
    const [topHeight, setTopHeight] = useState(0)
    const [headingHeight, setHeadingHeight] = useState(0)
    const onScroll = useMemo(
        () =>
            Animated.event([{ nativeEvent: { contentOffset: { y: scrollY } } }], {
                useNativeDriver: false,
            }),
        [scrollY]
    )
    const headingStyle = useMemo(
        () => ({
            position: 'absolute' as const,
            top: 0,
            left: 0,
            right: 0,
            zIndex: 3,
            // Hidden until the top area's height is known, so it never shows over it.
            opacity: topHeight > 0 ? 1 : 0,
            transform: [
                {
                    translateY: scrollY.interpolate({
                        inputRange: [0, Math.max(1, topHeight)],
                        outputRange: [topHeight, 0],
                        extrapolate: 'clamp',
                    }),
                },
            ],
        }),
        [scrollY, topHeight]
    )
    const stickyConfig = useMemo(
        () => ({ useNativeDriver: false as const, offset: headingHeight }),
        [headingHeight]
    )
    const onTopLayout = (e: LayoutChangeEvent) => setTopHeight(e.nativeEvent.layout.height)
    const onHeadingLayout = (e: LayoutChangeEvent) => setHeadingHeight(e.nativeEvent.layout.height)
    return { onScroll, headingStyle, stickyConfig, onTopLayout, onHeadingLayout }
}
