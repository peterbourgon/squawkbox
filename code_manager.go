package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
)

type codeManager struct {
	mtx      sync.Mutex
	filename string
}

var (
	errBadCode       = errors.New("bad code")
	errBadUseCount   = errors.New("use count must be > 0")
	errBadExpiration = errors.New("code expiration must be in the future")
	errCodeExists    = errors.New("code exists")
)

type bypassCode struct {
	Code      string `json:"code"`
	UseCount  int    `json:"use_count"`
	ExpiresAt string `json:"expires_at"` // RFC3339
}

const codeExt = ".code"

func newCodeManager(filename string) (*codeManager, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		if err := writeCodes(filename, map[string]bypassCode{}); err != nil {
			return nil, errors.Wrap(err, "couldn't create codes file")
		}
	}
	return &codeManager{
		filename: filename,
	}, nil
}

func (cm *codeManager) addCode(code string, useCount int, expiresAt time.Time) error {
	if !isNumeric(code) {
		return errBadCode
	}
	if useCount <= 0 {
		return errBadUseCount
	}
	if time.Now().After(expiresAt) {
		return errBadExpiration
	}

	cm.mtx.Lock()
	defer cm.mtx.Unlock()

	codes, err := readCodes(cm.filename)
	if err != nil {
		return errors.Wrap(err, "couldn't read codes file")
	}

	if _, ok := codes[code]; ok {
		return errCodeExists
	}

	codes[code] = bypassCode{
		Code:      code,
		UseCount:  useCount,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}

	if err := writeCodes(cm.filename, codes); err != nil {
		return errors.Wrap(err, "couldn't write codes file")
	}

	return nil
}

func (cm *codeManager) checkCode(code string) (err error) {
	cm.mtx.Lock()
	defer cm.mtx.Unlock()

	codes, err := readCodes(cm.filename)
	if err != nil {
		return errors.Wrap(err, "couldn't read codes file")
	}

	c, ok := codes[code]
	if !ok {
		return errBadCode
	}

	expiry, err := time.Parse(time.RFC3339, c.ExpiresAt)
	if err != nil {
		panic("bad code time format")
	}

	if time.Now().After(expiry) {
		delete(codes, code)
		if err := writeCodes(cm.filename, codes); err != nil {
			return errors.Wrap(err, "couldn't re-write codes file")
		}
		return errBadCode // expired
	}

	c.UseCount--
	if c.UseCount <= 0 {
		delete(codes, code)
	} else {
		codes[code] = c
	}
	if err := writeCodes(cm.filename, codes); err != nil {
		return errors.Wrap(err, "couldn't re-write codes file")
	}

	return nil
}

func (cm *codeManager) listCodes() (map[string]bypassCode, error) {
	cm.mtx.Lock()
	defer cm.mtx.Unlock()

	codes, err := readCodes(cm.filename)
	if err != nil {
		return map[string]bypassCode{}, errors.Wrap(err, "couldn't read codes file")
	}

	return codes, nil
}

func (cm *codeManager) revokeCode(code string) error {
	cm.mtx.Lock()
	defer cm.mtx.Unlock()

	codes, err := readCodes(cm.filename)
	if err != nil {
		return errors.Wrap(err, "couldn't read codes file")
	}

	if _, ok := codes[code]; !ok {
		return errBadCode
	}

	delete(codes, code)

	if err := writeCodes(cm.filename, codes); err != nil {
		return errors.Wrap(err, "couldn't re-write codes file")
	}

	return nil
}

func readCodes(filename string) (map[string]bypassCode, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return map[string]bypassCode{}, errors.Wrap(err, "couldn't open codes file")
	}

	codes := map[string]bypassCode{}
	if err := json.Unmarshal(buf, &codes); err != nil {
		return map[string]bypassCode{}, errors.Wrap(err, "couldn't unmarshal codes file")
	}

	return codes, nil
}

func writeCodes(filename string, codes map[string]bypassCode) error {
	buf, err := json.MarshalIndent(codes, "", "    ")
	if err != nil {
		return errors.Wrap(err, "couldn't marshal codes")
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, secureFileMode)
	if err != nil {
		return errors.Wrap(err, "couldn't create codes file")
	}
	defer f.Close()

	if _, err := f.Write(buf); err != nil {
		return errors.Wrap(err, "couldn't write codes file")
	}

	return nil
}
