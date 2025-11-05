package main

import (
	"slices"
	"strings"
)

func mentionUsers(ids []string) string {
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

func contains(arr []string, v string) bool {
	return slices.Contains(arr, v)
}

func remove(arr []string, v string) []string {
	out := arr[:0]
	for _, x := range arr {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
