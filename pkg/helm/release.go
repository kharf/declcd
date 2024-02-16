package helm

// ReleaseDeclaration is a Declaration of the desired state (Release) in a Git repository.
type ReleaseDeclaration struct {
	// Name influences the name of the installed objects of a Helm Chart.
	// When set, the installed objects are suffixed with the chart name.
	// Defaults to the chart name.
	Name string `json:"name"`
	// Namespace specifies the Kubernetes namespace to which the Helm Chart is installed to.
	// Defaults to default.
	Namespace string `json:"namespace"`
	Chart     Chart  `json:"chart"`
	Values    Values `json:"values"`
}

// Values provide a way to override Helm Chart template defaults with custom information.
type Values map[string]interface{}

// Release is a running instance of a Chart and the current state in a Kubernetes Cluster.
type Release struct {
	// Name of the installed objects of a Helm Chart.
	Name string `json:"name"`
	// Namespaces specifies the Kubernetes namespace where the Helm Chart is installed to.
	Namespace string `json:"namespace"`
	Chart     Chart  `json:"chart"`
	Values    Values `json:"values"`
	// Version is an int which represents the revision of the release.
	Version int `json:"-"`
}
