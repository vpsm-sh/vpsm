package auditlog

import "strings"

var sensitiveFlags = map[string]struct{}{
	"--token":      {},
	"--public-key": {},
	"--user-data":  {},
}

// SanitizeArgs redacts sensitive flag values for audit storage.
func SanitizeArgs(args []string) []string {
	sanitized := make([]string, 0, len(args))
	skipNext := false

	for _, arg := range args {
		if skipNext {
			sanitized = append(sanitized, "<redacted>")
			skipNext = false
			continue
		}

		if _, ok := sensitiveFlags[arg]; ok {
			sanitized = append(sanitized, arg)
			skipNext = true
			continue
		}

		if key, _, ok := strings.Cut(arg, "="); ok {
			if _, ok := sensitiveFlags[key]; ok {
				sanitized = append(sanitized, key+"=<redacted>")
				continue
			}
		}

		sanitized = append(sanitized, arg)
	}

	if skipNext {
		sanitized = append(sanitized, "<redacted>")
	}

	return sanitized
}
