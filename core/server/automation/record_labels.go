// tinycld/core/server/automation/record_labels.go
//
// Names the record a relation placeholder points at, so `{{delivered_to}}`
// in a rule's text renders "sales@example.com" rather than a record id. The
// name is the same one the client's record picker shows
// (core/lib/record-labels/label.ts renderRecordLabel): the label the target's
// package declares in its manifest's recordLabels, else the display field
// the catalog picks, else the id.
package automation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// RecordLabelDecl mirrors label.ts's RecordLabelDecl. The generator copies
// every manifest's recordLabels into automation_defs.json, keyed by
// collection name, after validating them, so Go only renders them.
type RecordLabelDecl struct {
	Format    string            `json:"format"`
	Relations map[string]string `json:"relations,omitempty"`
}

var labelPlaceholderRe = regexp.MustCompile(`\{([^{}]*)\}`)

// TemplateEnv is what a template may read beyond the trigger record: the
// app, to load the records relation fields point at, the declared labels to
// name them by, and the rule owner, whose view rule they must pass.
type TemplateEnv struct {
	App     core.App
	Labels  map[string]RecordLabelDecl
	OwnerID string
	// Dynamic holds a run's "fields.<key>" values.
	Dynamic map[string]any
	// Escape, when set, is applied to every value a placeholder puts into
	// the template (EscapeMarkdown for a markdown param).
	Escape func(string) string
}

// recordNamer names related records for one template substitution. It looks
// the owner up once, on first need, because most templates name no relation.
type recordNamer struct {
	env         TemplateEnv
	owner       *core.Record
	ownerLoaded bool
}

func (n *recordNamer) ownerMaySee(rec *core.Record) bool {
	if !n.ownerLoaded {
		n.ownerLoaded = true
		if owner, err := n.env.App.FindRecordById("users", n.env.OwnerID); err == nil {
			n.owner = owner
		}
	}
	if n.owner == nil {
		return false
	}
	info := &core.RequestInfo{Auth: n.owner}
	ok, err := n.env.App.CanAccessRecord(rec, info, rec.Collection().ViewRule)
	return err == nil && ok
}

// visibleRecord loads one record the owner may see. The engine reads as
// superuser, so without the view-rule check a placeholder could print a
// field of a record the owner cannot open in the app.
func (n *recordNamer) visibleRecord(collection, id string) *core.Record {
	if id == "" {
		return nil
	}
	rec, err := n.env.App.FindRecordById(collection, id)
	if err != nil || !n.ownerMaySee(rec) {
		return nil
	}
	return rec
}

// relationText renders every record a relation field points at, by name.
// A record that is gone, or that the owner may not see, renders as its id:
// that is what the placeholder rendered before relations had names, and it
// leaks nothing the trigger record does not already hold.
func (n *recordNamer) relationText(field *core.RelationField, record *core.Record) string {
	ids := record.GetStringSlice(field.Name)
	target, err := n.env.App.FindCachedCollectionByNameOrId(field.CollectionId)
	if err != nil {
		return strings.Join(ids, ", ")
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, n.recordName(target, id))
	}
	return strings.Join(names, ", ")
}

func (n *recordNamer) recordName(target *core.Collection, id string) string {
	rec := n.visibleRecord(target.Name, id)
	if rec == nil {
		return id
	}
	if decl, ok := n.env.Labels[target.Name]; ok {
		return n.declaredName(decl, rec)
	}
	if text := labelField(rec, displayFieldFor(target)); text != "" {
		return text
	}
	return id
}

// declaredName renders a RecordLabelDecl the way renderRecordLabel does: the
// literal text verbatim, each placeholder from the record or the one related
// record, and the id when every placeholder is empty.
func (n *recordNamer) declaredName(decl RecordLabelDecl, rec *core.Record) string {
	related := map[string]*core.Record{}
	hasValue := false
	text := labelPlaceholderRe.ReplaceAllStringFunc(decl.Format, func(m string) string {
		path := strings.Split(labelPlaceholderRe.FindStringSubmatch(m)[1], ".")
		var value string
		switch len(path) {
		case 1:
			value = labelField(rec, path[0])
		case 2:
			value = labelField(n.relatedRecord(decl, rec, path[0], related), path[1])
		}
		if value != "" {
			hasValue = true
		}
		return value
	})
	if !hasValue {
		return rec.Id
	}
	return text
}

// relatedRecord loads the record a label reads through, once per label.
func (n *recordNamer) relatedRecord(decl RecordLabelDecl, rec *core.Record, relation string, cache map[string]*core.Record) *core.Record {
	if cached, ok := cache[relation]; ok {
		return cached
	}
	var out *core.Record
	collection := decl.Relations[relation]
	if ids := rec.GetStringSlice(relation); collection != "" && len(ids) > 0 {
		out = n.visibleRecord(collection, ids[0])
	}
	cache[relation] = out
	return out
}

// labelField reads one field as label text: strings and numbers only, the
// same as label.ts's stringField, and never a hidden field.
func labelField(rec *core.Record, name string) string {
	if rec == nil {
		return ""
	}
	if name == "id" {
		return rec.Id
	}
	field := rec.Collection().Fields.GetByName(name)
	if field == nil || field.GetHidden() {
		return ""
	}
	switch v := rec.Get(name).(type) {
	case string:
		return v
	case float64, int, int64:
		return fmt.Sprint(v)
	default:
		return ""
	}
}
