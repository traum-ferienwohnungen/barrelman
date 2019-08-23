package utils

import (
	"fmt"
	"strings"
)

var (
	IgnoredNamespaces = &namespaceMap{
		// List namespaces ignored by default here
		"kube-system": struct{}{},
	}
)

type namespaceMap map[string]struct{}

func (n namespaceMap) String() string {
	ns := make([]string, len(n))
	i := 0
	for k := range n {
		ns[i] = k
		i++
	}
	return strings.Join(ns, ",")
}

func (n namespaceMap) Set(v string) error {
	if v == "" {
		return fmt.Errorf("empty string not allowed as namespace")
	}

	if strings.HasPrefix(v, "-") {
		// Remove strings prefixed with a dash from map
		delete(n, strings.TrimPrefix(v, "-"))
	} else {
		// Add everything else
		n[v] = struct{}{}
	}
	return nil
}

func (n namespaceMap) IsIgnored(ns string) bool {
	_, ignored := n[ns]
	return ignored
}
