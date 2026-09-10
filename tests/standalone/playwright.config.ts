import { defineConfig, devices } from '@playwright/test'

// Self-contained config for the single-binary self-host smoke test.
//
// Deliberately NOT under tests/e2e/: that tree's config owns a webServer that
// resets the DB, runs `expo export`, and promotes a bundle (up to 240s) before
// any spec runs. This suite boots the SHIPPED binary instead — its own process,
// its own data dir, its own port — so the shared stack would be both irrelevant
// and in the way. Same reasoning as tests/install/, which talks to an
// already-running container.
//
// The spec builds and supervises the binary itself, so there is no webServer
// here either.
export default defineConfig({
    testDir: '.',
    testMatch: '**/*.spec.ts',
    fullyParallel: false,
    workers: 1,
    retries: 0,
    // The build is a cold cross-package Go compile on a clean cache.
    timeout: 300_000,
    expect: { timeout: 30_000 },
    reporter: 'list',
    use: {
        ...devices['Desktop Chrome'],
        trace: 'retain-on-failure',
        screenshot: 'only-on-failure',
    },
})
