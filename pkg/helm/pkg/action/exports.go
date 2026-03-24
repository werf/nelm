package action

import "k8s.io/client-go/kubernetes"

const NotesFileSuffix = notesFileSuffix

var (
	NewSecretClient    = newSecretClient
	NewConfigMapClient = newConfigMapClient
)

func NewLazyClient(namespace string, clientFn func() (*kubernetes.Clientset, error)) *lazyClient {
	return &lazyClient{
		namespace: namespace,
		clientFn:  clientFn,
	}
}
