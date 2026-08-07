package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Generic helpers over the PocketBase record REST API. Package-level
// functions rather than methods because Go methods cannot be generic.

type ListOptions struct {
	Filter  string
	Sort    string
	Fields  string
	Page    int
	PerPage int
	// SkipTotal skips the server-side COUNT; TotalItems/TotalPages come back
	// as -1. Use it for paging loops that only care about "was the page full".
	SkipTotal bool
}

type ListResult[T any] struct {
	Page       int `json:"page"`
	PerPage    int `json:"perPage"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
	Items      []T `json:"items"`
}

func ListRecords[T any](ctx context.Context, c *Client, collection string, o ListOptions) (ListResult[T], error) {
	q := url.Values{}
	if o.Filter != "" {
		q.Set("filter", o.Filter)
	}
	if o.Sort != "" {
		q.Set("sort", o.Sort)
	}
	if o.Fields != "" {
		q.Set("fields", o.Fields)
	}
	if o.Page > 0 {
		q.Set("page", strconv.Itoa(o.Page))
	}
	if o.PerPage > 0 {
		q.Set("perPage", strconv.Itoa(o.PerPage))
	}
	if o.SkipTotal {
		q.Set("skipTotal", "1")
	}
	path := recordsPath(collection)
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out ListResult[T]
	err := c.GetJSON(ctx, path, &out)
	return out, err
}

// ListAll pages through every record matching filter. Callers that expect
// unbounded result sets should prefer ListRecords with an explicit limit.
func ListAll[T any](ctx context.Context, c *Client, collection, filter, sort string) ([]T, error) {
	const perPage = 200
	var all []T
	for page := 1; ; page++ {
		res, err := ListRecords[T](ctx, c, collection, ListOptions{
			Filter: filter, Sort: sort, Page: page, PerPage: perPage, SkipTotal: true,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, res.Items...)
		if len(res.Items) < perPage {
			return all, nil
		}
	}
}

func GetRecord[T any](ctx context.Context, c *Client, collection, id string) (T, error) {
	var out T
	err := c.GetJSON(ctx, recordsPath(collection)+"/"+url.PathEscape(id), &out)
	return out, err
}

func CreateRecord[T any](ctx context.Context, c *Client, collection string, body any) (T, error) {
	var out T
	err := c.PostJSON(ctx, recordsPath(collection), body, &out)
	return out, err
}

func UpdateRecord[T any](ctx context.Context, c *Client, collection, id string, body any) (T, error) {
	var out T
	err := c.PatchJSON(ctx, recordsPath(collection)+"/"+url.PathEscape(id), body, &out)
	return out, err
}

func DeleteRecord(ctx context.Context, c *Client, collection, id string) error {
	return c.Delete(ctx, recordsPath(collection)+"/"+url.PathEscape(id))
}

// FileURL builds the origin-relative URL of a stored file. filename must be
// the record's stored (sanitized, suffixed) file name — never the display
// name.
func FileURL(collection, recordID, filename string) string {
	return "/api/files/" + url.PathEscape(collection) + "/" +
		url.PathEscape(recordID) + "/" + url.PathEscape(filename)
}

func recordsPath(collection string) string {
	return "/api/collections/" + url.PathEscape(collection) + "/records"
}

// Filter substitutes each {:name} placeholder in expr with the quoted value,
// using the same quoting as PocketBase's own pb.filter helper (and the web
// client's inline quote()): strings get `\` and `"` escaped and are wrapped
// in double quotes; numbers and bools render bare.
func Filter(expr string, params map[string]any) string {
	for name, v := range params {
		expr = strings.ReplaceAll(expr, "{:"+name+"}", filterValue(v))
	}
	return expr
}

func filterValue(v any) string {
	switch t := v.(type) {
	case string:
		s := strings.ReplaceAll(t, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		return `"` + s + `"`
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return filterValue(fmt.Sprint(t))
	}
}
