package cli

import "strings"

type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}
