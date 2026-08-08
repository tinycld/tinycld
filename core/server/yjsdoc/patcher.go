package yjsdoc

import (
	ycrdt "github.com/skyterra/y-crdt"
)

// InstallPatcher subscribes a `beforeObserverCalls` listener that walks the
// transaction's Changed / ChangedParentTypes maps and initializes any type
// whose EH/DEH/Map are nil.
//
// This works around a y-crdt quirk: NewYXmlElement — invoked from
// content_type.go::readYXmlElement while decoding an inbound update — returns
// an element with PrelimAttrs set but EH/DEH/Map left nil. When transaction
// cleanup then fires observers on that newly inserted element,
// CallTypeObservers → CallEventHandlerListeners panics dereferencing nil EH.
//
// `beforeObserverCalls` runs after the read phase has populated Changed and
// before the loop that calls CallObserver on each modified type, so patching
// here guarantees every observer-firing path sees a valid EventHandler.
//
// Moved verbatim from text/server/runtime.go. It is a workaround for a library
// defect, not code with an intent of its own — tidying it is how the panics
// come back.
func InstallPatcher(doc *Doc) {
	handler := ycrdt.NewObserverHandler(func(args ...interface{}) {
		if len(args) == 0 {
			return
		}
		trans, ok := args[0].(*ycrdt.Transaction)
		if !ok || trans == nil {
			return
		}
		for t := range trans.Changed {
			patchTypeAndAncestors(t)
		}
		for t := range trans.ChangedParentTypes {
			patchTypeAndAncestors(t)
		}
	})
	doc.On("beforeObserverCalls", handler)
}

// patchTypeAndAncestors patches t and every ancestor.
//
// The deep-observe phase fires DEH listeners on a changed type's PARENTS, and a
// parent minted during inbound decode can have a nil DEH while never appearing
// as a key in Changed / ChangedParentTypes — an attribute set on a deeply
// nested element deep-observes up to an ancestor the direct walk missed, and
// GetDEH() returns nil → panic. The depth cap guards against a cycle; real
// trees are shallow.
func patchTypeAndAncestors(t interface{}) {
	at, ok := t.(ycrdt.IAbstractType)
	for depth := 0; ok && at != nil && depth < 256; depth++ {
		patchAbstractType(at)
		at = at.Parent()
	}
}

// patchAbstractType initializes EH/DEH/Map on the embedded AbstractType. The
// branch list mirrors the typeRefs table in y-crdt's content_type.go.
func patchAbstractType(t interface{}) {
	switch v := t.(type) {
	case *ycrdt.YXmlElement:
		ensureAbstractTypeInitialized(&v.AbstractType)
	case *ycrdt.YXmlFragment:
		ensureAbstractTypeInitialized(&v.AbstractType)
	case *ycrdt.YXmlText:
		ensureAbstractTypeInitialized(&v.AbstractType)
	case *ycrdt.YText:
		ensureAbstractTypeInitialized(&v.AbstractType)
	case *ycrdt.YArray:
		ensureAbstractTypeInitialized(&v.AbstractType)
	case *ycrdt.YMap:
		ensureAbstractTypeInitialized(&v.AbstractType)
	case *ycrdt.YXmlHook:
		ensureAbstractTypeInitialized(&v.AbstractType)
	}
}

func ensureAbstractTypeInitialized(at *ycrdt.AbstractType) {
	if at.EH == nil {
		at.EH = ycrdt.NewEventHandler()
	}
	if at.DEH == nil {
		at.DEH = ycrdt.NewEventHandler()
	}
	if at.Map == nil {
		at.Map = make(map[string]*ycrdt.Item)
	}
}
