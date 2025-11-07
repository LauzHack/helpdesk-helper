// Package utils/utils.go
package utils

import (
	"strings"
)

func MentionUsers(ids []string) string {
	if len(ids) == 0 {
		return "(none)"
	}
	var parts []string
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			parts = append(parts, "<@"+id+">")
		}
	}
	return strings.Join(parts, " ")
}
