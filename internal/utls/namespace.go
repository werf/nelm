package utls

import "k8s.io/apimachinery/pkg/apis/meta/v1"

func FallbackNamespace(namespace string, fallbackNamespaces ...string) string {
	if namespace != "" {
		return namespace
	}

	if len(fallbackNamespaces) > 0 {
		for _, ns := range fallbackNamespaces {
			if ns != "" {
				return ns
			}
		}
	}

	return v1.NamespaceDefault
}
