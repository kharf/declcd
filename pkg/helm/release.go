package helm

type Release struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Chart     Chart  `json:"chart"`
	Values    Values `json:"values"`
}
