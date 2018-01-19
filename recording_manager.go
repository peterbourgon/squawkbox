package main

import (
	"bytes"
	"io"
	"net/http"
	"sort"
	"sync"

	"github.com/pkg/errors"
)

type recordingManager struct {
	mtx        sync.Mutex
	recordings map[string]*bytes.Buffer
}

func newRecordingManager() *recordingManager {
	return &recordingManager{
		recordings: map[string]*bytes.Buffer{},
	}
}

func (rm *recordingManager) saveRecording(name, url string) error {
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
	rm.recordings[name] = &buf
	return nil
}

func (rm *recordingManager) listRecordings() []string {
	rm.mtx.Lock()
	defer rm.mtx.Unlock()
	res := make([]string, 0, len(rm.recordings))
	for name := range rm.recordings {
		res = append(res, name)
	}
	sort.Strings(res)
	return res
}

func (rm *recordingManager) getRecording(name string) (io.Reader, error) {
	rm.mtx.Lock()
	defer rm.mtx.Unlock()
	buf, ok := rm.recordings[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return bytes.NewReader(buf.Bytes()), nil
}
