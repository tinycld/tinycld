# Vendored sobek Node.js compatibility modules

These packages are vendored from
[github.com/ohayocorp/sobek_nodejs](https://github.com/ohayocorp/sobek_nodejs)
(MIT licensed, see `LICENSE`), a sobek port of `github.com/dop251/goja_nodejs`.

Only the subpackages required by the jsvm plugin are vendored:
`require`, `console`, `process`, `buffer`, plus their internal
dependencies `util`, `goutil`, and `errors`.

The only local modification is rewriting the internal import paths from
`github.com/ohayocorp/sobek_nodejs/<pkg>` to
`github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/<pkg>`.
`github.com/grafana/sobek` remains an external dependency.
