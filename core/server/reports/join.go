package reports

import (
	"fmt"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

// bindValuePath binds a filter's or formula's path: to-one links, then a
// field. A link to a source CheckSources refused for app is unknown.
func bindValuePath(app core.App, src *Source, path Path, at string, kind pathKind) (*pathPlan, error) {
	b := &binder{app: app, invalid: map[string]validation.Error{}, unknown: map[string]validation.Error{}}
	p := b.path(src, path, at, kind, "")
	if err := b.err(); err != nil {
		return nil, err
	}
	return p, nil
}

func paramOf(c *compiler, id, at string) (paramValue, error) {
	v, ok := c.params[id]
	if !ok {
		return paramValue{}, unknownRef(at, "unknown_param", fmt.Sprintf("the report has no param %q", id))
	}
	return v, nil
}

func unknownColumn(at, id string) *SpecError {
	return unknownRef(at, "unknown_column", fmt.Sprintf("the report has no column %q", id))
}

// enter guards a formula column against reading itself.
func enter(visiting map[string]bool, id, at string) (func(), error) {
	if visiting[id] {
		return nil, invalid(at, fmt.Sprintf("the formula of %q reads itself", id))
	}
	visiting[id] = true
	return func() { delete(visiting, id) }, nil
}

// joinCtx compiles inside w's own select: the filter, a group key, the
// pivot key. Paths join from the scope's base after prefix (an exists where
// reads the linked rows' paths). It reads row values only: no totals and no
// running values.
type joinCtx struct {
	ctxBase
	r        *rowsCTE
	sc       *scope
	prefix   []hopPlan
	src      *Source
	isFilter bool
	visiting map[string]bool
}

func (j *joinCtx) col(id, at string) (expr, error) {
	if len(j.prefix) > 0 || j.sc != j.r.root {
		return expr{}, invalid(at, "a where reads paths of the linked rows, not report columns")
	}
	cp := j.r.c.p.byID[id]
	if cp == nil {
		return expr{}, unknownColumn(at, id)
	}
	done, err := enter(j.visiting, id, at)
	if err != nil {
		return expr{}, err
	}
	defer done()
	return j.r.rowValue(cp, at, j.visiting)
}

func (j *joinCtx) path(p Path, at string) (expr, error) {
	bound, err := bindValuePath(j.r.c.p.app, j.src, p, at, pathValue)
	if err != nil {
		return expr{}, err
	}
	hops := append(append([]hopPlan{}, j.prefix...), bound.hops...)
	return j.sc.value(&pathPlan{hops: hops, field: bound.field}), nil
}

// record is refused: the filter, a group key and a fill read base rows
// before any group has a record (R21).
func (j *joinCtx) record(_ string, _ Path, at string) (expr, error) {
	return expr{}, invalid(at, "the filter, a group key and a fill read rows before they are grouped: read the field through a path")
}

func (j *joinCtx) param(id, at string) (expr, error) {
	v, err := paramOf(j.r.c, id, at)
	if err != nil {
		return expr{}, err
	}
	return paramExpr(j, v), nil
}

func (j *joinCtx) aggregate(op Aggregate, _ *Node, _ int, at string) (expr, error) {
	return expr{}, invalid(at, fmt.Sprintf("%s totals rows: use it in a column, not in a filter or a group", op))
}

func (j *joinCtx) window(op string, _ *Node, _ int, at string) (expr, error) {
	return expr{}, invalid(at, op+" reads the rows a report shows: use it in a column")
}

func (j *joinCtx) exists(n *Node, at string) (expr, error) {
	if !j.isFilter {
		return expr{}, invalid(at, n.Op+" belongs in the filter")
	}
	link, err := bindValuePath(j.r.c.p.app, j.src, n.Link, at+".link", pathLinks)
	if err != nil {
		return expr{}, err
	}
	mk := func(sc *scope, hops []hopPlan) fctx {
		src := sc.src
		if len(hops) > 0 {
			src = hops[len(hops)-1].to
		}
		return &joinCtx{r: j.r, sc: sc, prefix: hops, src: src, isFilter: true, visiting: j.visiting}
	}
	return j.sc.exists(j.prefix, link, n.Where, mk, at)
}

func (j *joinCtx) bind(v any) string                       { return j.sc.s.bind(v) }
func (j *joinCtx) fiscalStart() string                     { return j.bind(int64(j.r.c.fiscal)) }
func (j *joinCtx) filter() bool                            { return j.isFilter }
func (j *joinCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(j, j.r, n, at) }
