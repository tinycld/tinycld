import fs from 'node:fs'
import path from 'node:path'
import { defineConfig, type PlaywrightTestConfig } from '@playwright/test'
import {
    combineMemberConfigs,
    type LoadedMember,
    type MemberSpecs,
    SPECS_ENV,
} from './multi-member'

// One Playwright run over spec files of several members: one server pair, one
// fixture seed, one report. `tinycld-pkg test:e2e <spec file>...` writes the
// members and their files to SPECS_ENV and runs Playwright with this config.
const raw = process.env[SPECS_ENV]
if (!raw) throw new Error(`${SPECS_ENV} is not set; run through \`tinycld-pkg test:e2e <files>\``)
const members: MemberSpecs[] = JSON.parse(raw)

async function loadConfig(dir: string): Promise<PlaywrightTestConfig> {
    return (await import(path.join(dir, 'playwright.config.ts'))).default
}

const loaded: LoadedMember[] = []
for (const member of members) {
    const config = await loadConfig(member.dir)
    // A member points testDir through a node_modules symlink; the files arrive
    // as real paths, so compare them against the real test dir.
    const realTestDir = config.testDir ? fs.realpathSync(config.testDir) : member.dir
    loaded.push({ ...member, config, realTestDir })
}

export default defineConfig(combineMemberConfigs(loaded))
