package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tinycld.org/cli/client"
)

func TestExplainListsEveryFieldErrorAndTheHint(t *testing.T) {
	body := []byte(`{"status":400,"message":"Failed to create record.","data":{
		"definition.fields.2.key":{"code":"validation_invalid_definition","message":"\"Email\" is not a valid field key."},
		"definition.fields.0.kind":{"code":"validation_invalid_definition","message":"\"phone\" is not a field kind."}}}`)
	err := explain(&client.APIError{Status: 400, Message: "Failed to create record.", Body: body}, "tinycld form create --help")
	msg := err.Error()
	if exitCode(err) != 1 {
		t.Errorf("exit code = %d, want 1", exitCode(err))
	}
	i0 := strings.Index(msg, "definition.fields.0.kind")
	i2 := strings.Index(msg, "definition.fields.2.key")
	if i0 < 0 || i2 < 0 || i0 > i2 {
		t.Errorf("both paths, sorted: %q", msg)
	}
	if !strings.Contains(msg, "See: tinycld form create --help") {
		t.Errorf("the hint line: %q", msg)
	}
}

func TestExplainNestedFieldErrors(t *testing.T) {
	body := []byte(`{"status":400,"message":"x","data":{"actions":{"0":{"ref":{"code":"c","message":"no installed action"}}}}}`)
	msg := explain(&client.APIError{Status: 400, Body: body}, "").Error()
	if !strings.Contains(msg, "actions.0.ref: no installed action") {
		t.Errorf("nested data is flattened: %q", msg)
	}
}

func TestExplainMissingScopeSaysToSignInAgain(t *testing.T) {
	err := explain(&client.APIError{Status: 403, Message: "Requires the forms scope"}, "")
	if !strings.Contains(err.Error(), "tinycld auth login") {
		t.Errorf("a missing scope says how to fix it: %q", err)
	}
}

func TestExplainLeavesOtherErrorsAlone(t *testing.T) {
	plain := errors.New("network down")
	if explain(plain, "x") != plain {
		t.Error("a non-API error is returned as it is")
	}
}

func TestReadJSONFileNamesLineAndColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "def.json")
	if err := os.WriteFile(path, []byte("{\n  \"version\": 1,\n  \"pages\": [}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readJSONFile(path)
	if err == nil || !strings.Contains(err.Error(), "def.json:3:") {
		t.Fatalf("got %v, want a def.json:3:<col> error", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("a bad file is a usage error, got exit %d", exitCode(err))
	}
}
