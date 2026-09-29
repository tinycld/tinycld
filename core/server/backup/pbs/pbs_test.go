package pbs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gopbs "github.com/osshield/gopbs/pbs"
	"github.com/osshield/gopbs/pbstest"

	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/repo/repotest"
)

func configFor(t *testing.T, srv *pbstest.Server, key string) json.RawMessage {
	t.Helper()
	c := srv.Config()
	raw, _ := json.Marshal(Config{
		Server: c.BaseURL, Fingerprint: c.Fingerprint, Datastore: c.Datastore,
		AuthID: "test@pbs!test", Secret: "secret", Key: key, BackupID: "acme.example",
	})
	return raw
}

func TestPBSMeetsTheContract(t *testing.T) {
	repotest.Run(t, func(t *testing.T) repo.Repository {
		r, err := Open(configFor(t, pbstest.NewServer(t), ""))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}, repotest.Options{Dedup: true})
}

func TestPBSMeetsTheContractEncrypted(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	repotest.Run(t, func(t *testing.T) repo.Repository {
		r, err := Open(configFor(t, pbstest.NewServer(t), key))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}, repotest.Options{Dedup: true})
}

func TestInterruptedPutLeavesNoSnapshot(t *testing.T) {
	srv := pbstest.NewServer(t)
	r, _ := Open(configFor(t, srv, ""))
	srv.DropAfterBytes(1024)
	if _, err := r.Put(context.Background(), repotest.Fixture(t), nil); err == nil {
		t.Fatal("put succeeded through a dropped connection")
	}
	if n := len(srv.Snapshots()); n != 0 {
		t.Fatalf("%d snapshots kept", n)
	}
}

func TestBadTokenErrorHidesTheSecret(t *testing.T) {
	srv := pbstest.NewServer(t)
	srv.RejectAuth(true)
	r, _ := Open(configFor(t, srv, ""))
	_, err := r.List(context.Background())
	if !errors.Is(err, gopbs.ErrAuth) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigRequiresFields(t *testing.T) {
	for _, raw := range []string{`{}`, `{"server":"pbs.example"}`, `{"server":"pbs.example","datastore":"s","auth_id":"a@pbs!t"}`} {
		if _, err := ParseConfig(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	c, err := ParseConfig(json.RawMessage(`{"server":"pbs.example","datastore":"s","auth_id":"a@pbs!t","secret":"x","backup_id":"acme"}`))
	if err != nil || c.baseURL() != "https://pbs.example:8007" || c.Host() != "pbs.example" {
		t.Fatalf("%+v %v", c, err)
	}
}

// TestAgainstARealServer runs the contract against a real PBS when
// TINYCLD_TEST_PBS holds a Config JSON (without backup_id). It is for a
// manual check before a release; CI runs the pbstest suite above.
func TestAgainstARealServer(t *testing.T) {
	raw := os.Getenv("TINYCLD_TEST_PBS")
	if raw == "" {
		t.Skip("TINYCLD_TEST_PBS is not set")
	}
	var c Config
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	c.BackupID = "tinycld-test-" + strconv.FormatInt(time.Now().Unix(), 10)
	cfg, _ := json.Marshal(c)
	repotest.Run(t, func(t *testing.T) repo.Repository {
		r, err := Open(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}, repotest.Options{Dedup: true})
}
