package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func render(vars map[string]string, format string) (string, error) {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	switch format {
	case formatExports:
		for _, k := range keys {
			fmt.Fprintf(&b, "export %s=%s\n", k, shellQuote(vars[k]))
		}
	case formatDotenv:
		for _, k := range keys {
			fmt.Fprintf(&b, "%s=%s\n", k, dotenvQuote(vars[k]))
		}
	case formatJSON:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(vars); err != nil {
			return "", err
		}
		return buf.String(), nil
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
	return b.String(), nil
}

// shellQuote quotes v as a $'...' string (bash, zsh, busybox ash, POSIX.1-2024)
// that contains no whitespace or glob characters. The result therefore stays
// byte-exact under both `eval "$(aws-env)"` and the unquoted `eval $(aws-env)`,
// where word splitting and pathname expansion apply.
func shellQuote(v string) string {
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\'':
			b.WriteString(`\'`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\t':
			b.WriteString(`\t`)
		case shellSafe(c):
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, `\x%02x`, c)
		}
	}
	b.WriteString("'")
	return b.String()
}

func shellSafe(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c >= 0x80: // UTF-8 bytes are neither IFS nor glob characters
		return true
	}
	return strings.IndexByte("_-.,/:@%+=^", c) >= 0
}

// dotenvQuote produces a double-quoted value for dotenv parsers. dotenv has no
// single spec, so this is best effort; use --format json or exec for exact values.
func dotenvQuote(v string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`$`, `\$`,
		"\n", `\n`,
		"\r", `\r`,
	)
	return `"` + r.Replace(v) + `"`
}
