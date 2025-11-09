// Package kubernetes provides Kubernetes client and resource management functionality.
package kubernetes

// Config holds the Kubernetes-specific configuration
type Config struct {
	Namespace     string
	Deployment    string
	ConfigMapName string
}
