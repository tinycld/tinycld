import * as path from 'node:path'
import type { PlaywrightTestConfig } from '@playwright/test'

export const SPECS_ENV = 'TINYCLD_E2E_SPECS'

// What the CLI hands the combined config: a member and the real, absolute
// paths of its spec files to run.
export interface MemberSpecs {
    dir: string
    name: string
    files: string[]
}

export interface LoadedMember extends MemberSpecs {
    config: PlaywrightTestConfig
    realTestDir: string
}

type Project = NonNullable<PlaywrightTestConfig['projects']>[number]

function escapeRegExp(text: string): string {
    return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// A project name prefix from the package name: `@tinycld/books` → `books`.
function slugOf(name: string): string {
    return name.split('/').pop() ?? name
}

// Each named file as a pattern on its path under the member's test dir. A
// pattern on that relative path matches whichever route Playwright walked to
// the file (the real dir, or the node_modules symlink a member config uses),
// and cannot select a file of the same name in another member.
function fileMatchers(member: LoadedMember): RegExp[] {
    return member.files.map(file => {
        const relative = path.relative(member.realTestDir, file)
        if (relative.startsWith('..') || path.isAbsolute(relative)) {
            throw new Error(
                `${file} is not under the test dir of ${member.name} (${member.realTestDir})`
            )
        }
        const parts = relative.split(path.sep).map(escapeRegExp)
        return new RegExp(`[\\\\/]${parts.join('[\\\\/]')}$`)
    })
}

// The servers and the seed belong to the whole run, so every member must ask
// for the same ones. A member with its own stack cannot join a combined run.
function sharedStack(config: PlaywrightTestConfig): string {
    return JSON.stringify([config.webServer ?? null, config.globalSetup ?? null])
}

function memberProjects(member: LoadedMember): Project[] {
    const slug = slugOf(member.name)
    const { config } = member
    const testMatch = fileMatchers(member)
    const own = config.projects?.length ? config.projects : [{}]
    return own.map(project => ({
        ...project,
        name: project.name ? `${slug}:${project.name}` : slug,
        testDir: project.testDir ?? config.testDir,
        timeout: project.timeout ?? config.timeout,
        dependencies: project.dependencies?.map(dependency => `${slug}:${dependency}`),
        testMatch,
    }))
}

// One config for all the members: the run-level settings (servers, seed,
// reporter, workers, browser) come from the first member, which every member
// inherits them from, and each member becomes one project (or its own
// projects, prefixed) that keeps its test dir, timeout and filters.
export function combineMemberConfigs(members: LoadedMember[]): PlaywrightTestConfig {
    const [first, ...rest] = members
    if (!first) throw new Error('No spec files to run')
    const stack = sharedStack(first.config)
    for (const member of rest) {
        if (sharedStack(member.config) !== stack) {
            throw new Error(
                `${member.name} starts its own servers or global setup, so it cannot run together with ${first.name}. Run its specs in a separate command.`
            )
        }
    }
    const { testDir: _testDir, timeout: _timeout, projects: _projects, ...runLevel } = first.config
    return { ...runLevel, projects: members.flatMap(memberProjects) }
}
