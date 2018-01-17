package main

import "testing"

func TestParseAuthData(t *testing.T) {
	for _, testcase := range []struct {
		name  string
		input string
		realm string
		user  string
		pass  string
		err   error
	}{
		{"empty",
			"",
			"", "", "", errBadAuthData,
		},
		{"basic",
			"realm:user:pass",
			"realm", "user", "pass", nil,
		},
		{"utf8",
			"réalm:u_s_e_r:p·a·ß",
			"réalm", "u_s_e_r", "p·a·ß", nil,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			realm, user, pass, err := parseAuthData([]byte(testcase.input))
			if realm != testcase.realm || user != testcase.user || pass != testcase.pass || err != testcase.err {
				t.Fatalf(
					"want %q/%q/%q/%v, have %q/%q/%q/%v",
					testcase.realm, testcase.user, testcase.pass, testcase.err,
					realm, user, pass, err,
				)
			}
		})
	}
}

func TestParseForwardNumber(t *testing.T) {
	for _, testcase := range []struct {
		name   string
		input  string
		digits string
		err    error
	}{
		{"empty",
			"",
			"", errBadForwardNumber,
		},
		{"basic",
			"12345",
			"12345", nil,
		},
		{"other chars",
			"ré 1 lm2-345u_s_e_r:p·a·ß-6789",
			"123456789", nil,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			digits, err := parseForwardNumber([]byte(testcase.input))
			if digits != testcase.digits || err != testcase.err {
				t.Fatalf(
					"want %q/%v, have %q/%v",
					testcase.digits, testcase.err,
					digits, err,
				)
			}
		})
	}
}
