package format

import (
	"io"

	"filippo.io/age"
)

type MemberReport struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	OK   bool   `json:"ok"`
}

type Report struct {
	Members []MemberReport `json:"members"`
	OK      bool           `json:"ok"`
}

// Inspect reads a whole archive, verifying every member, without writing
// anything. It is what `tinycld backup inspect` runs.
func Inspect(r io.Reader, identity age.Identity) (Manifest, Report, error) {
	rd, err := NewReader(r, identity)
	if err != nil {
		return Manifest{}, Report{}, err
	}
	defer rd.Close()
	m, err := rd.ReadManifest()
	if err != nil {
		return Manifest{}, Report{}, err
	}
	var rep Report
	for {
		hdr, body, err := rd.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return m, rep, err
		}
		if _, err := io.Copy(io.Discard, body); err != nil {
			return m, rep, err
		}
		rep.Members = append(rep.Members, MemberReport{Name: hdr.Name, Size: hdr.Size})
	}
	verr := rd.Verify()
	rep.OK = verr == nil
	for i := range rep.Members {
		rep.Members[i].OK = rd.sums[rep.Members[i].Name] == rd.recorded[rep.Members[i].Name]
	}
	return m, rep, verr
}
