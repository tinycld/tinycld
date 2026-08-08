---
title: Searching across packages
summary: Find anything from anywhere with the / palette
tags: [search, keyboard, packages]
order: 20
---

Press `/` anywhere in the app to open the search palette. It opens already
scoped to whatever package you're looking at, so searching the current
package costs nothing extra. `/` typed inside a text field types a literal
slash instead — the palette only opens when nothing else is capturing your
keystrokes.

## Searching everywhere

Press ⌫ (backspace) with an empty search box to remove the scope chip and
search every installed package at once. With no chip, results are one flat
list ordered by match quality, with a small badge on each row showing which
package it came from.

## Scoping to a package

Type a package name followed by a colon — `drive:` — to turn it into a chip
that limits the search to that package. The name only becomes a chip once you
type the colon, so you can still search for the word itself: typing `mail`
with no colon finds anything containing the word "mail", chip or no chip.

Add a second chip to search exactly those packages together:

    drive: mail: budget

That finds anything matching "budget" in Drive or Mail, grouped by package,
and nothing else. With two or more chips the badge disappears from each
row — you already know which package you're looking at from the group it's
under.

## Excluding words

Put a minus sign in front of a word to exclude it:

    budget -draft

A hyphen inside a word is left alone, so a term like `q1-report` still
searches for it as written.

The palette does not support `AND`, `OR`, quotes, or parentheses — every word
you type has to match, and excluded words never do. That's a deliberate
limit, not a gap: plain words plus `:` for scope and `-` for exclusion cover
what search-as-you-type needs.

## Keys

- `↑` `↓` — move through results
- `↵` — open the selected result
- `⌫` — remove the last scope chip (only when the box is otherwise empty)
- `esc` — close the palette without opening anything

## From the command line

The [command line tool](help://core:command-line) searches the same packages
with the same grammar, so a query that works in the palette works in a
terminal unchanged:

```
tinycld search "drive: mail: budget -draft"
```

Quote the query so your shell keeps it as one argument. Scope chips also have
a flag form, which you can mix with chips:

```
tinycld search budget --pkg drive --pkg mail
```

Exclusions have a flag form too — `--not draft` is the same as writing
`-draft` inside the query. A query that *starts* with a hyphen has to come
after `--`, otherwise the terminal reads it as an option:

```
tinycld search -- "-draft budget"
```

Results print as a table of package, title, subtitle, and detail. After them,
a summary line reports how many matches each package holds. Two notes worth
reading: if a package couldn't be reached, it's named as a warning — its rows
are missing, not absent — and if a package holds more matches than were
shown, it's named too, so a short list is never mistaken for the whole answer.

Add `--json` for scripting. It prints the raw response, which carries extra
per-package values the table has no column for, and keeps the summary out of
the output so tools like `jq` read it cleanly.
