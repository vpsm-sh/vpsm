package services

import (
	"strings"

	"nathanbeddoewebdev/vpsm/internal/util"
)

func cacheKey(provider string, parts ...string) string {
	values := make([]string, 0, len(parts)+1)
	if provider != "" {
		values = append(values, util.NormalizeKey(provider))
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, util.NormalizeKey(part))
	}
	if len(values) == 0 {
		return "dns"
	}
	return strings.Join(values, "_")
}
