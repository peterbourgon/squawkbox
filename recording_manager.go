package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/pkg/errors"
)

type recordingManager struct {
	mtx sync.Mutex
	dir string
}

func newRecordingManager(dir string) *recordingManager {
	return &recordingManager{
		dir: dir,
	}
}

func (rm *recordingManager) saveRecording(name string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "fetching recording")
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return errors.Wrap(err, "downloading recording")
	}

	rm.mtx.Lock()
	defer rm.mtx.Unlock()
	filename := filepath.Join(rm.dir, name)
	return ioutil.WriteFile(filename, buf.Bytes(), secureFileMode)
}

func (rm *recordingManager) listRecordings() []string {
	rm.mtx.Lock()
	defer rm.mtx.Unlock()

	matches, err := filepath.Glob(filepath.Join(rm.dir, "*.wav"))
	if err != nil {
		return []string{}
	}

	for i := range matches {
		matches[i] = filepath.Base(matches[i])
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	return matches
}

func (rm *recordingManager) getRecording(name string) (io.Reader, error) {
	rm.mtx.Lock()
	defer rm.mtx.Unlock()
	filename := filepath.Join(rm.dir, name)
	return os.Open(filename)
}
