// Package utils/utils.go
package utils

import (
	"slices"
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

func Contains(arr []string, v string) bool {
	return slices.Contains(arr, v)
}

func Remove(arr []string, v string) []string {
	out := arr[:0]
	for _, x := range arr {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
