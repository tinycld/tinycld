// Test-only stub for expo-router/head. The real module re-exports
// react-helmet-async whose CJS entry contains JSX that Vite's SSR node
// environment cannot parse. Unit tests don't observe document.title
// effects; this stub renders nothing and exposes the `default export`
// shape (the Head component) that callers consume.

const Head = (_props: { children?: unknown }) => null

export default Head
