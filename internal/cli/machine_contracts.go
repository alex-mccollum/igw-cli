package cli

import "sort"

type exitCodeEntry struct {
	Name string
	Code int
}

func stableExitCodeMap() map[string]int {
	return map[string]int{
		"ok":      0,
		"usage":   2,
		"auth":    6,
		"network": 7,
	}
}

func stableExitCodeEntries() []exitCodeEntry {
	raw := stableExitCodeMap()
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	entries := make([]exitCodeEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, exitCodeEntry{Name: key, Code: raw[key]})
	}
	return entries
}
