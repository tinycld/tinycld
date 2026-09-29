package archive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"

	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/repo/repotest"
)

// memStore keeps each archive in memory by ref.
type memStore struct {
	mu   sync.Mutex
	objs map[repo.Ref]*bytes.Buffer
}

type bufCloser struct{ *bytes.Buffer }

func (bufCloser) Close() error { return nil }

func TestArchiveMeetsTheContract(t *testing.T) {
	repotest.Run(t, func(t *testing.T) repo.Repository {
		id, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		st := &memStore{objs: map[repo.Ref]*bytes.Buffer{}}
		return &Repository{
			Recipient: id.Recipient(),
			Identity:  id,
			Level:     zstd.SpeedFastest,
			Target: func(_ context.Context, created time.Time) (io.WriteCloser, repo.Ref, error) {
				ref := repo.Ref(created.Format(time.RFC3339Nano))
				buf := &bytes.Buffer{}
				st.mu.Lock()
				st.objs[ref] = buf
				st.mu.Unlock()
				return bufCloser{buf}, ref, nil
			},
			Open: func(_ context.Context, ref repo.Ref) (io.ReadCloser, error) {
				st.mu.Lock()
				defer st.mu.Unlock()
				buf, ok := st.objs[ref]
				if !ok {
					return nil, fmt.Errorf("archive: no object for ref %q", ref)
				}
				return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
			},
		}
	}, repotest.Options{})
}
