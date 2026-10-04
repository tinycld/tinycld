package coreserver

import (
	"errors"
	"testing"

	"tinycld.org/core/installjob"
)

func TestCreateInstallLogRecordsTriggerAndChanges(t *testing.T) {
	app := adminConsoleTestApp(t)
	job := installjob.New("version_change", "mail", "")
	job.Trigger = "auto"
	job.Changes = []installjob.VersionChange{{Slug: "mail", TargetVersion: "0.6.0"}}

	rec := createInstallLog(app, job, "version_change")
	if rec == nil {
		t.Fatal("no install log row")
	}
	if rec.GetString("trigger") != "auto" {
		t.Errorf("trigger = %q", rec.GetString("trigger"))
	}
	var got []installjob.VersionChange
	if err := rec.UnmarshalJSONField("changes", &got); err != nil || len(got) != 1 || got[0].TargetVersion != "0.6.0" {
		t.Errorf("changes = %+v, %v", got, err)
	}

	manual := createInstallLog(app, installjob.New("install", "x", ""), "install")
	if manual.GetString("trigger") != "manual" {
		t.Errorf("default trigger = %q", manual.GetString("trigger"))
	}
}

func TestBeginVersionChangeRefusesWhileBusy(t *testing.T) {
	busy := installjob.New("install", "x", "")
	if _, ok := installjob.Claim(busy); !ok {
		t.Fatal("could not claim")
	}
	t.Cleanup(func() { installjob.Release(busy) })

	_, err := beginVersionChange(nil, []installjob.VersionChange{{Slug: "mail", TargetVersion: "0.6.0"}}, "auto")
	if !errors.Is(err, errJobBusy) {
		t.Fatalf("err = %v, want errJobBusy", err)
	}
}
