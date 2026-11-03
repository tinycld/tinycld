package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"tinycld.org/cli/client"
)

type fieldProblem struct {
	Path    string
	Message string
}

// fieldProblems flattens PocketBase's error data ({"a.b": {code, message}}
// or nested maps) into one problem per path, sorted by path.
func fieldProblems(body []byte) []fieldProblem {
	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &resp) != nil {
		return nil
	}
	var out []fieldProblem
	var walk func(prefix string, raw json.RawMessage)
	walk = func(prefix string, raw json.RawMessage) {
		var leaf struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &leaf) == nil && leaf.Message != "" {
			out = append(out, fieldProblem{Path: prefix, Message: leaf.Message})
			return
		}
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) != nil {
			return
		}
		for k, v := range nested {
			walk(prefix+"."+k, v)
		}
	}
	for k, v := range resp.Data {
		walk(k, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// explain turns a refused write into lines an agent can act on: every field
// problem with its path, then the help to read. Other errors pass through.
func explain(err error, hint string) error {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.Status == 403 && strings.Contains(apiErr.Message, "Requires the") {
		return Failed(fmt.Errorf("%s. Your sign-in does not include it (it may predate this command). Run: tinycld auth login", apiErr.Message))
	}
	problems := fieldProblems(apiErr.Body)
	if len(problems) == 0 {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "the server refused the change (%d problem(s)):", len(problems))
	for _, p := range problems {
		fmt.Fprintf(&b, "\n  %s: %s", p.Path, p.Message)
	}
	if hint != "" {
		fmt.Fprintf(&b, "\nSee: %s", hint)
	}
	return Failed(errors.New(b.String()))
}

// readJSONFile reads a --from file and refuses one that is not JSON before
// any request, naming the line and column of the first error.
func readJSONFile(path string) (json.RawMessage, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, Usage(fmt.Errorf("%s: %w", path, err))
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		var offset int64 = -1
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syn):
			offset = syn.Offset
		case errors.As(err, &typ):
			offset = typ.Offset
		}
		if offset < 0 {
			return nil, Usage(fmt.Errorf("%s: %w", path, err))
		}
		line, col := lineCol(b, offset)
		return nil, Usage(fmt.Errorf("%s:%d:%d: %w", path, line, col, err))
	}
	return json.RawMessage(bytes.TrimSpace(b)), nil
}

func lineCol(b []byte, offset int64) (int, int) {
	if offset > int64(len(b)) {
		offset = int64(len(b))
	}
	before := b[:offset]
	line := bytes.Count(before, []byte("\n")) + 1
	col := int(offset) - bytes.LastIndexByte(before, '\n')
	return line, col
}
