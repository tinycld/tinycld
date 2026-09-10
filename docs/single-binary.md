# Running TinyCld from a single binary

A self-host release ships one binary per platform. It contains the server, the
web interface, and every bundled feature package. It needs no Docker, no Node,
and no Go toolchain.

## Install

Each release carries a binary per platform — `tinycld-linux-amd64`,
`tinycld-linux-arm64`, `tinycld-darwin-amd64` (Intel Mac), and
`tinycld-darwin-arm64` (Apple silicon) — alongside a `SHA256SUMS` file.
Download the one for your platform plus `SHA256SUMS` from the
[latest release](https://github.com/tinycld/tinycld/releases/latest), then
verify and run it:

```sh
sha256sum --check --ignore-missing SHA256SUMS
chmod +x tinycld-linux-amd64
./tinycld-linux-amd64 serve
```

On first run it creates `./tinycld-data/`, applies its migrations, listens on
`127.0.0.1:8090`, and prints a setup URL with a one-time token. Open that URL
to create the first account.

## Options

These are PocketBase's own flags — the binary adds none of its own, so anything
the upstream `serve` command accepts works here too.

| Flag | Default | Purpose |
| --- | --- | --- |
| `--dir` | `./tinycld-data/pb_data` | Database and uploads |
| `--http` | `127.0.0.1:8090` | HTTP listen address |
| `--https` | unset | HTTPS address; enables automatic certificates |

The default listen address is loopback, so a first run is not exposed to the
network before you have created an account. To serve publicly, pass
`--http 0.0.0.0:8090` or put a reverse proxy in front.

To keep state somewhere other than the working directory:

```sh
./tinycld serve --dir /srv/tinycld/pb_data
```

Generated state that is not the database itself (the release and build
directories) is kept beside `--dir`, in its parent — so the single directory
you back up is `/srv/tinycld` in the example above.

## TLS

`--https` uses automatic certificates, which requires binding ports 80 and 443.
Either grant the capability or run behind a reverse proxy:

```sh
sudo setcap cap_net_bind_service=+ep ./tinycld
```

## Backups

Everything is under the data directory. Stop the server and copy it, or
snapshot the database alone while it runs:

```sh
sqlite3 ./tinycld-data/pb_data/data.db "VACUUM INTO './backup.db'"
```

## Upgrading

Download the new binary, replace the old one, and restart. Migrations apply
automatically at startup. Back up the data directory first.

## Differences from the Docker distribution

- **No in-app package installation.** The feature set is fixed when the binary
  is built, so the Packages screen's install and upgrade actions are not
  available — the API behind them is not served at all. Adding or changing
  packages means taking a new binary (or using the Docker distribution, which
  can rebuild itself). You can still enable and disable bundled features in
  Settings.
- **Editing collections in the admin UI does not write migration files.** There
  is nowhere to write them, and `migrate create` / `migrate collections` are
  not registered. Schema changes belong in a development workspace. This is the
  one behavioral difference that affects an existing workflow.
- **Hook files are not watched.** The hooks are compiled in, so there is no
  file to change and no auto-restart.
- **Everything is one process.** There is no separate build or release
  directory to manage.
