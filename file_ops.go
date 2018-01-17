package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/pkg/errors"
)

const secureFileMode = 0600

func readSecureFile(filename string) ([]byte, error) {
	if filename == "" {
		return nil, errNoFile
	}
	fi, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if mode := fi.Mode(); mode != secureFileMode {
		return nil, errBadMode
	}
	return ioutil.ReadFile(filename)
}

var (
	errNoFile  = errors.New("no filename provided")
	errBadMode = errors.New("insecure file mode; need chmod 600")
)

func parseAuthFile(filename string) (realm, user, pass string, err error) {
	buf, err := readSecureFile(filename)
	if err != nil {
		return "", "", "", errors.Wrap(err, "parsing auth file")
	}
	return parseAuthData(bytes.TrimSpace(buf))
}

func parseForwardFile(filename string) (digits string, err error) {
	buf, err := readSecureFile(filename)
	if err != nil {
		return "", errors.Wrap(err, "parsing forward number file")
	}
	return parseForwardNumber(buf)
}

var (
	authDataRegex  = regexp.MustCompile(`([^:]+):([^:]+):([^:]+)`)
	errBadAuthData = errors.New(`bad auth data; need "realm:user:pass"`)
)

func parseAuthData(data []byte) (realm, user, pass string, err error) {
	matches := authDataRegex.FindAllStringSubmatch(string(data), 3)
	if matches == nil {
		return "", "", "", errBadAuthData
	}
	if len(matches[0]) != 1+3 {
		return "", "", "", errBadAuthData
	}
	return matches[0][1], matches[0][2], matches[0][3], nil
}

var (
	errBadForwardNumber = errors.New(`bad forward number; need e.g. "1-212-555-0199"`)
)

func parseForwardNumber(data []byte) (digits string, err error) {
	for _, b := range data {
		switch b {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			digits += string(b)
		default:
			continue
		}
	}
	if digits == "" {
		return "", errBadForwardNumber
	}
	return digits, nil
}
