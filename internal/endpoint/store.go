package endpoint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forms"
)

// store appends submissions to one JSON-lines file per form: a plain file,
// readable only by the endpoint's user, that a person (or later the editor)
// can read. It is a record, not a database: nothing here ever queries it.
type store struct{ mu sync.Mutex }

func (st *store) append(dir string, s *forms.Submission, now time.Time) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, s.Form.Name+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(map[string]any{"time": now.UTC().Format(time.RFC3339), "form": s.Form.Name, "values": s.Values})
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}
