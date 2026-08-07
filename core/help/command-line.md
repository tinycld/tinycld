---
title: Command line tool
summary: Download the tinycld CLI, get past the unsigned-binary warnings, and log in from a terminal.
tags: [cli, terminal, automation, download]
order: 42
---

The `tinycld` command line tool gives you terminal and script access to your
server — the commands it offers match exactly the packages installed here.

## Downloading

Open **Settings → Personal → About** and find **Command line tools**. Pick the
build for your computer — on a Mac, choose **Apple Silicon** for M-series
machines and **Intel** for older ones.

After downloading, make the file executable:

```
chmod +x tinycld
```

## First run on macOS and Windows

The binaries are unsigned in this version, so the first run trips each
platform's safety check:

- **macOS** blocks the app with "cannot be opened". Clear the quarantine flag
  and it runs normally:

  ```
  xattr -d com.apple.quarantine ./tinycld
  ```

- **Windows** shows a SmartScreen warning. Choose **More info**, then
  **Run anyway**.

## Logging in

```
tinycld auth login {{server-host}}
```

The tool shows a short one-time code and opens your browser. Sign in if you
need to, check the code matches, and approve the device. Your credentials are
stored in the operating system keychain, and the login appears under
[Connected apps](help://core:connected-apps) — revoke it there any time to
sign that terminal out remotely.

## Everyday use

- `tinycld --help` lists every command your server's packages provide.
- `tinycld auth status` shows who you are logged in as.
- `tinycld context list` shows your saved servers; `tinycld auth login`
  against a second host adds another and switches to it.
- `tinycld auth logout` revokes this device's access and forgets its
  credentials.

Every command accepts `--json` for scripting, and prompts are skipped with
`--yes` so commands run cleanly in CI.

## Package commands

Each installed package contributes its own command group. See
[Drive from the command line](help://drive:command-line) for working with
files (`tinycld drive ls`, `put`, `get`, …) and
[Mail from the command line](help://mail:command-line) for searching,
reading, and sending mail (`tinycld mail search`, `read`, `send`, …).
