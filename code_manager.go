package main

import (
	"errors"
	"sync"
	"time"
)

type codeManager struct {
	mtx   sync.Mutex
	codes map[string]bypassCode
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

func newCodeManager() *codeManager {
	return &codeManager{
		codes: map[string]bypassCode{},
	}
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
	if _, ok := cm.codes[code]; ok {
		return errCodeExists
	}
	cm.codes[code] = bypassCode{
		Code:      code,
		UseCount:  useCount,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}
	return nil
}

func (cm *codeManager) checkCode(code string) error {
	cm.mtx.Lock()
	defer cm.mtx.Unlock()
	c, ok := cm.codes[code]
	if !ok {
		return errBadCode // invalid code
	}
	expires, err := time.Parse(time.RFC3339, c.ExpiresAt)
	if err != nil {
		panic("bad time format: " + code + ": " + c.ExpiresAt)
	}
	if time.Now().After(expires) {
		delete(cm.codes, code)
		return errBadCode // expired code
	}
	c.UseCount--
	if c.UseCount <= 0 {
		delete(cm.codes, code) // code has no more uses
	} else {
		cm.codes[code] = c // code has more uses
	}
	return nil // valid code
}

func (cm *codeManager) listCodes() map[string]bypassCode {
	cm.mtx.Lock()
	defer cm.mtx.Unlock()
	codes := map[string]bypassCode{}
	for k, v := range cm.codes {
		codes[k] = v
	}
	return codes
}

func (cm *codeManager) revokeCode(code string) error {
	cm.mtx.Lock()
	defer cm.mtx.Unlock()
	if _, ok := cm.codes[code]; !ok {
		return errBadCode
	}
	delete(cm.codes, code)
	return nil
}
