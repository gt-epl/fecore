package pkg

const (
	// DefaultFunctionNamespace is the default containerd namespace functions are created
	DefaultFunctionNamespace = "faasedge-fn"

	// NamespaceLabel indicates that a namespace is managed by faasedge-core
	NamespaceLabel = "faasedge"

	// FaasEdgeNamespace is the containerd namespace services are created
	FaasEdgeNamespace = "faasedge"

	faasServicesPullAlways = false

	defaultSnapshotter = "overlayfs"
)
