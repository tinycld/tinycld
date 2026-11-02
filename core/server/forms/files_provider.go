package forms

import (
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
)

// registerFilesProviders lets rule actions read a host record's files
// (automation.TriggerFiles) through the host's own files contract.
func registerFilesProviders(defs *Defs) {
	for i := range defs.Hosts {
		host := &defs.Hosts[i]
		if host.Files == nil {
			continue
		}
		files := *host.Files
		automation.RegisterFilesProvider(host.Collection, func(app core.App, record *core.Record) ([]automation.TriggerFile, error) {
			rows, err := app.FindRecordsByFilter(files.Collection, files.RecordColumn+" = {:id}", "", 0, 0, map[string]any{"id": record.Id})
			if err != nil {
				return nil, err
			}
			out := make([]automation.TriggerFile, 0, len(rows))
			for _, r := range rows {
				out = append(out, automation.TriggerFile{
					Field: r.GetString(files.FieldColumn), Name: r.GetString(files.NameColumn),
					Size: int64(r.GetInt(files.SizeColumn)), Record: r, FileField: files.FileColumn,
				})
			}
			return out, nil
		})
	}
}
