// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

// Package prefix holds the method-name prefix rules that the interceptors,
// MissingAuthLevelsFromRegistry and protoauthztest share, so they normalise,
// validate and match prefixes the same way.
package prefix

import (
	"fmt"
	"strings"
)

// Normalise gives each prefix exactly one leading "/" and panics on any
// prefix that can match no method name.
func Normalise(prefixes []string) []string {
	normalised := make([]string, len(prefixes))
	for i, prefix := range prefixes {
		normalised[i] = "/" + strings.TrimLeft(prefix, "/")
		if !valid(normalised[i]) {
			panic(fmt.Sprintf(`protoauthz: invalid prefix %q: a prefix may hold only ASCII letters, digits, "_", "." and "/", and after normalising to one leading "/", no "//" and at most two "/"`, prefix))
		}
	}
	return normalised
}

// Under reports whether fullMethod is covered by prefixes: true when there
// are none, else true only under one of them. prefixes must come from
// Normalise.
func Under(fullMethod string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(fullMethod, prefix) {
			return true
		}
	}
	return false
}

// valid reports whether a normalised prefix has the shape of the start of
// "/package.Service/Method": only ASCII letters, digits, "_", "." and "/",
// no "//", and at most two "/". Normalise has already given it exactly one
// leading "/", so a "//" anywhere means a third "/", and the count covers it.
func valid(prefix string) bool {
	if strings.Count(prefix, "/") > 2 {
		return false
	}
	for i := range len(prefix) {
		switch c := prefix[i]; {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '_', c == '.', c == '/':
		default:
			return false
		}
	}
	return true
}
