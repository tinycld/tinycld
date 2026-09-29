package apis

import (
	"encoding/json"
	"log/slog"
	"reflect"
	"strings"

	"github.com/ganigeorgiev/fexpr"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/routine"
	"github.com/pocketbase/pocketbase/tools/search"
	"github.com/pocketbase/pocketbase/tools/store"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
	"golang.org/x/sync/errgroup"
)

// Realtime "leave" events.
//
// A subscription with a filter (or a collection with a list/view rule) only
// receives the writes that still match after the write. Nothing tells the
// subscriber that a record it holds stopped matching, so its view goes stale.
//
// The only way to know a record *left* a subscription is to know whether it
// matched *before* the write. That is answered here the same way the delete
// path answers it: an OnModelUpdate handler runs the access check against the
// committed (pre-write) row and parks the result on the client, and the
// post-commit broadcast diffs it against the post-write check. A subscription
// that matched before and not after receives a "delete" event carrying only
// the record id, so a row hidden by a rule change leaks none of its new values.
//
// The pre-write check is skipped for every subscription whose filter and rule
// read none of the fields this write changes: other tables are identical at
// both moments, so the answer cannot differ from the post-write check.

// realtimeLeaveRecord is the payload of a leave event. It deliberately carries
// no field values.
type realtimeLeaveRecord struct {
	Id             string `json:"id"`
	CollectionId   string `json:"collectionId"`
	CollectionName string `json:"collectionName"`
}

// realtimeExprRoots caches, per filter/rule string, the leading segment of
// each identifier it references ("rel.name" -> "rel", "tags:length" -> "tags").
var realtimeExprRoots = store.New(make(map[string][]string, 50))

func realtimeLeaveKey(model core.Model) string {
	return getDryCacheKey("leave", model)
}

// realtimeCacheLeaveCandidates records, before the row is written, which of
// the client subscriptions currently see the record and could stop seeing it
// because of this write.
//
// accessCheckApp must read committed data (the outer, non-transactional app)
// so that the checks see the pre-write row even when the update runs inside
// a transaction.
func realtimeCacheLeaveCandidates(app core.App, record *core.Record, accessCheckApp core.App) error {
	chunks := app.SubscriptionsBroker().ChunkedClients(clientsChunkSize)
	if len(chunks) == 0 {
		return nil // no subscribers
	}

	changed := realtimeChangedFields(record)
	if len(changed) == 0 {
		return nil
	}

	subscriptionRuleMap := realtimeSubscriptionRuleMap(record)
	key := realtimeLeaveKey(record)

	group := new(errgroup.Group)

	for _, chunk := range chunks {
		group.Go(routine.SafeWrap(func() error {
			for _, client := range chunk {
				visible := map[string]struct{}{}

				for prefix, rule := range subscriptionRuleMap {
					subs := client.Subscriptions(prefix)
					if len(subs) == 0 {
						continue
					}

					clientAuth, _ := client.Get(RealtimeClientAuthKey).(*core.Record)

					for sub, options := range subs {
						filter := options.Query[search.FilterQueryParam]
						if !realtimeVisibilityDependsOn(rule, filter, changed) {
							continue
						}

						requestInfo := &core.RequestInfo{
							Context: core.RequestInfoContextRealtime,
							Method:  "GET",
							Query:   options.Query,
							Headers: options.Headers,
							Auth:    clientAuth,
						}

						if realtimeCanAccessRecord(accessCheckApp, record, requestInfo, rule) {
							visible[sub] = struct{}{}
						}
					}
				}

				if len(visible) > 0 {
					client.Set(key, visible)
				}
			}

			return nil
		}))
	}

	return group.Wait()
}

// realtimeTakeLeaveCandidates removes and returns the subscriptions parked on
// the client by realtimeCacheLeaveCandidates for this record.
func realtimeTakeLeaveCandidates(client subscriptions.Client, record *core.Record) map[string]struct{} {
	key := realtimeLeaveKey(record)

	visible, _ := client.Get(key).(map[string]struct{})
	if visible != nil {
		client.Unset(key)
	}

	return visible
}

// realtimeSendLeaveMessages sends a "delete" event to every subscription that
// saw the record before the write and no longer does.
func realtimeSendLeaveMessages(app core.App, client subscriptions.Client, subs map[string]struct{}, record *core.Record) {
	if len(subs) == 0 {
		return
	}

	collection := record.Collection()

	data, err := json.Marshal(&recordData{
		Action: "delete",
		Record: realtimeLeaveRecord{
			Id:             record.Id,
			CollectionId:   collection.Id,
			CollectionName: collection.Name,
		},
	})
	if err != nil {
		app.Logger().Debug(
			"[broadcastRecord] leave data marshal error",
			slog.String("id", record.Id),
			slog.String("collectionName", collection.Name),
			slog.String("error", err.Error()),
		)
		return
	}

	for sub := range subs {
		// the client may have resubscribed between the two checks
		if !client.HasSubscription(sub) {
			continue
		}

		msg := subscriptions.Message{Name: sub, Data: data}

		routine.FireAndForget(func() {
			client.Send(msg)
		})
	}
}

// realtimeSubscriptionRuleMap maps every subscription topic prefix that can
// deliver the record to the API rule that governs it.
func realtimeSubscriptionRuleMap(record *core.Record) map[string]*string {
	collection := record.Collection()

	return map[string]*string{
		(collection.Name + "/" + record.Id + "?"): collection.ViewRule,
		(collection.Id + "/" + record.Id + "?"):   collection.ViewRule,
		(collection.Name + "/*?"):                 collection.ListRule,
		(collection.Id + "/*?"):                   collection.ListRule,

		// @deprecated: the same as the wildcard topic but kept for backward compatibility
		(collection.Name + "?"): collection.ListRule,
		(collection.Id + "?"):   collection.ListRule,
	}
}

// realtimeChangedFields returns the names of the collection fields whose
// value differs from the record's original (last loaded) state.
//
// Autodate fields with OnUpdate are always reported as changed: their value is
// assigned by the write interceptor, after the OnModelUpdate hooks have run.
func realtimeChangedFields(record *core.Record) map[string]struct{} {
	original := record.Original()
	changed := map[string]struct{}{}

	for _, field := range record.Collection().Fields {
		name := field.GetName()

		if autodate, ok := field.(*core.AutodateField); ok && autodate.OnUpdate {
			changed[name] = struct{}{}
			continue
		}

		if !reflect.DeepEqual(original.Get(name), record.Get(name)) {
			changed[name] = struct{}{}
		}
	}

	return changed
}

// realtimeVisibilityDependsOn reports whether the record's visibility under
// the rule and filter can differ before and after a write that changed the
// given fields.
//
// A nil rule admits superusers only and a blank rule admits everyone; in both
// cases only the filter can change the answer.
func realtimeVisibilityDependsOn(rule *string, filter string, changed map[string]struct{}) bool {
	if rule != nil && realtimeExprReadsAny(*rule, changed) {
		return true
	}

	return filter != "" && realtimeExprReadsAny(filter, changed)
}

func realtimeExprReadsAny(expr string, fields map[string]struct{}) bool {
	for _, root := range realtimeExprIdentifierRoots(expr) {
		if _, ok := fields[root]; ok {
			return true
		}
	}

	return false
}

// realtimeExprIdentifierRoots returns the leading path segment of every
// non-@ identifier in the expression. Identifiers starting with "@" are
// request or cross-collection references, whose value does not change with
// the current record. An expression that fails to parse cannot match a record
// at all, so it reads nothing.
func realtimeExprIdentifierRoots(expr string) []string {
	if expr == "" {
		return nil
	}

	if roots, ok := realtimeExprRoots.GetOk(expr); ok {
		return roots
	}

	groups, err := fexpr.Parse(expr)
	if err != nil {
		return nil
	}

	seen := map[string]struct{}{}
	collectExprIdentifierRoots(groups, seen)

	roots := make([]string, 0, len(seen))
	for root := range seen {
		roots = append(roots, root)
	}

	realtimeExprRoots.SetIfLessThanLimit(expr, roots, 500)

	return roots
}

func collectExprIdentifierRoots(groups []fexpr.ExprGroup, into map[string]struct{}) {
	for _, group := range groups {
		switch item := group.Item.(type) {
		case fexpr.Expr:
			addExprIdentifierRoot(item.Left, into)
			addExprIdentifierRoot(item.Right, into)
		case fexpr.ExprGroup:
			collectExprIdentifierRoots([]fexpr.ExprGroup{item}, into)
		case []fexpr.ExprGroup:
			collectExprIdentifierRoots(item, into)
		}
	}
}

func addExprIdentifierRoot(token fexpr.Token, into map[string]struct{}) {
	if token.Type != fexpr.TokenIdentifier || strings.HasPrefix(token.Literal, "@") {
		return
	}

	root := token.Literal
	if i := strings.IndexAny(root, ".:"); i >= 0 {
		root = root[:i]
	}

	into[root] = struct{}{}
}

// bindRealtimeLeaveEvents binds the hooks that record, before an update, which
// subscriptions can see the record, so that realtimeBroadcastRecord can send a
// delete to the subscriptions it leaves.
func bindRealtimeLeaveEvents(app core.App) {
	// update: remember which subscriptions see the record before it changes
	app.OnModelUpdate().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			record := realtimeResolveRecord(e.App, e.Model, "")
			if record != nil {
				// note: use the outside scoped app instance so that the checks
				// read the committed row even when the update runs in a transaction
				err := realtimeCacheLeaveCandidates(e.App, record, app)
				if err != nil {
					app.Logger().Debug(
						"Failed to cache record leave candidates",
						slog.String("id", record.Id),
						slog.String("collectionName", record.Collection().Name),
						slog.String("error", err.Error()),
					)
				}
			}

			return e.Next()
		},
		Priority: 99, // execute as later as possible
	})

	// update: failure
	app.OnModelAfterUpdateError().Bind(&hook.Handler[*core.ModelErrorEvent]{
		Func: func(e *core.ModelErrorEvent) error {
			collection := realtimeResolveRecordCollection(e.App, e.Model)
			if collection != nil {
				err := realtimeUnsetDryCacheKey(e.App, realtimeLeaveKey(e.Model))
				if err != nil {
					app.Logger().Debug(
						"Failed to cleanup record leave candidates after update failure",
						slog.Any("id", e.Model.PK()),
						slog.String("collectionName", collection.Name),
						slog.String("error", err.Error()),
					)
				}
			}

			return e.Next()
		},
		Priority: -99,
	})
}
