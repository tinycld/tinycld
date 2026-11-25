package reports

import (
	"maps"
	"slices"
	"strings"
)

// noteReads records the collections a statement of this read selected rows
// from (stmt.rules; a union source is each of its parts). Every statement that
// is sent passes here: runner.query, and the two statements of a choice
// param's options (optionsPage, readOptionGroups).
func (c *compiler) noteReads(s *stmt) {
	for _, name := range s.rules {
		key, union := strings.CutPrefix(name, unionRulePrefix)
		if !union {
			c.noteCollection(name)
			continue
		}
		// An unregistered union notes nothing: stmt.ruleRead already fails the
		// statement for it, so the read never answers.
		if src := lookupRegistered(key); src != nil {
			for _, part := range src.collections() {
				c.noteCollection(part)
			}
		}
	}
}

func (c *compiler) noteCollection(name string) {
	if c.dataRead == nil {
		c.dataRead = map[string]bool{}
	}
	c.dataRead[name] = true
}

// noteHooks records the collections src's hooks read (Source.Watch). The
// caller runs a hook of src (Heading, FiscalStartMonth, Currency) and puts
// what it gives in the read's answer.
func (c *compiler) noteHooks(src *Source) {
	for _, w := range src.Watch {
		c.noteCollection(w)
	}
}

// collectionsRead are the collections noted up to now, sorted.
func (c *compiler) collectionsRead() []string {
	return slices.Sorted(maps.Keys(c.dataRead))
}
