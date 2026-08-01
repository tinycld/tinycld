#!/usr/bin/env node
// Minimal npm registry for the HOSTED install runner (run-hosted-install.sh).
//
// The multi-org builder fetches member specs with `npm pack --registry <url>`
// and discovers versions with `npm view --registry <url>` (MT_NPM_REGISTRY →
// builder.Config.NpmRegistry). This server fronts exactly that surface for a
// fixed fixture set packed at startup:
//
//   --pack <dir>            npm-pack a local checkout (one version, from its
//                           package.json) — the tinycld base sibling.
//   --pack-git <url>#<tag>  clone <url> at <tag> (depth 1) and npm-pack it —
//                           the @tinycld/todo fixture tags. Repeatable; same
//                           package name accumulates versions into one
//                           packument, which is what the Versions UI lists.
//
// It is the runnable sibling of startLocalNpmRegistry in
// multi-org/internal/controlplane/hosted_e2e_test.go, with two differences a
// browser-driven run needs: multiple versions per name, and a standalone
// process a bash runner can start/stop. ONLY member fetches hit this server —
// the build workspace's own pnpm install keeps normal registry resolution, so
// serving just the members is sufficient by design.
//
// Protocol served (all `npm pack`/`npm view` need):
//   GET /<name>                       → packument (dist-tags.latest = highest
//                                       semver; versions map with dist URLs)
//   GET /tarballs/<name>/<version>.tgz → the packed tarball
//
// Prints one line per packed fixture (`PACKED <name>@<version>`) and, once
// listening, `REGISTRY_URL=http://127.0.0.1:<port>` — the runner scrapes that.
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import fs from 'node:fs'
import http from 'node:http'
import os from 'node:os'
import path from 'node:path'

function fail(msg) {
    console.error(`hosted-npm-registry: ${msg}`)
    process.exit(1)
}

// ---------- argv ----------
const packDirs = []
const packGits = []
let port = 0
{
    const args = process.argv.slice(2)
    for (let i = 0; i < args.length; i++) {
        switch (args[i]) {
            case '--pack':
                packDirs.push(args[++i])
                break
            case '--pack-git':
                packGits.push(args[++i])
                break
            case '--port':
                port = Number(args[++i])
                break
            default:
                fail(`unknown argument ${args[i]}`)
        }
    }
    if (packDirs.length === 0 && packGits.length === 0) {
        fail('nothing to serve — pass --pack and/or --pack-git')
    }
}

const workRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'hosted-npm-registry-'))
process.on('exit', () => {
    fs.rmSync(workRoot, { recursive: true, force: true })
})

// packages: name → version → { tarball: Buffer, shasum, integrity }
const packages = new Map()

function npmPack(packDir) {
    // Absolute, or npm reads a relative path as the owner/repo github shorthand.
    const dir = path.resolve(packDir)
    const outDir = fs.mkdtempSync(path.join(workRoot, 'pack-'))
    // Filename is stdout's only line; npm's "notice" chatter goes to stderr.
    const out = execFileSync('npm', ['pack', dir, '--pack-destination', outDir], {
        encoding: 'utf8',
    })
    const lines = out.trim().split(/\s+/)
    const tgzName = lines[lines.length - 1]
    const tarball = fs.readFileSync(path.join(outDir, tgzName))

    const pkgJson = JSON.parse(fs.readFileSync(path.join(dir, 'package.json'), 'utf8'))
    const { name, version } = pkgJson
    if (!name || !version) fail(`unreadable package.json name/version in ${dir}`)

    if (!packages.has(name)) packages.set(name, new Map())
    if (packages.get(name).has(version)) {
        fail(`duplicate ${name}@${version} — two fixtures claim the same version`)
    }
    packages.get(name).set(version, {
        tarball,
        shasum: createHash('sha1').update(tarball).digest('hex'),
        integrity: `sha512-${createHash('sha512').update(tarball).digest('base64')}`,
    })
    console.log(`PACKED ${name}@${version}`)
}

for (const dir of packDirs) {
    if (!fs.existsSync(path.join(dir, 'package.json'))) fail(`--pack ${dir}: no package.json`)
    npmPack(dir)
}
for (const spec of packGits) {
    const hash = spec.lastIndexOf('#')
    if (hash <= 0) fail(`--pack-git ${spec}: expected <url>#<tag>`)
    const url = spec.slice(0, hash)
    const tag = spec.slice(hash + 1)
    const cloneDir = fs.mkdtempSync(path.join(workRoot, 'clone-'))
    execFileSync('git', ['clone', '--quiet', '--depth', '1', '--branch', tag, url, cloneDir], {
        stdio: ['ignore', 'ignore', 'inherit'],
    })
    npmPack(cloneDir)
}

// ---------- semver ordering for dist-tags.latest ----------
function semverKey(v) {
    // Enough for fixture versions (x.y.z, optional prerelease which sorts last
    // by the simple "release beats prerelease" rule).
    const [release, pre] = v.split('-')
    const nums = release.split('.').map(Number)
    return { nums, pre: pre ?? null }
}
function semverCompare(a, b) {
    const ka = semverKey(a)
    const kb = semverKey(b)
    for (let i = 0; i < 3; i++) {
        const d = (ka.nums[i] ?? 0) - (kb.nums[i] ?? 0)
        if (d !== 0) return d
    }
    if (ka.pre === kb.pre) return 0
    if (ka.pre === null) return 1
    if (kb.pre === null) return -1
    return ka.pre < kb.pre ? -1 : 1
}

// ---------- server ----------
const server = http.createServer((req, res) => {
    const url = decodeURIComponent(new URL(req.url, 'http://x').pathname).replace(/^\//, '')

    const tarballMatch = url.match(/^tarballs\/(.+)\/([^/]+)\.tgz$/)
    if (tarballMatch) {
        const entry = packages.get(tarballMatch[1])?.get(tarballMatch[2])
        if (!entry) {
            res.writeHead(404).end('not found')
            return
        }
        res.writeHead(200, { 'Content-Type': 'application/octet-stream' })
        res.end(entry.tarball)
        return
    }

    const byVersion = packages.get(url)
    if (!byVersion) {
        res.writeHead(404, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ error: 'Not found' }))
        return
    }
    const name = url
    const versions = {}
    for (const [version, entry] of byVersion) {
        versions[version] = {
            name,
            version,
            dist: {
                tarball: `http://127.0.0.1:${server.address().port}/tarballs/${name}/${version}.tgz`,
                shasum: entry.shasum,
                integrity: entry.integrity,
            },
        }
    }
    const latest = [...byVersion.keys()].sort(semverCompare).pop()
    res.writeHead(200, { 'Content-Type': 'application/json' })
    res.end(
        JSON.stringify({
            name,
            'dist-tags': { latest },
            versions,
        })
    )
})

server.listen(port, '127.0.0.1', () => {
    console.log(`REGISTRY_URL=http://127.0.0.1:${server.address().port}`)
})
