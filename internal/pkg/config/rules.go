package config

type MatchLabels struct {
	ExportService string `yaml:"export-service"`
}

type MatchExpressions struct {
}

type LabelSelectors struct {
	MatchLabels      MatchLabels      `yaml:"matchLabels,omitempty"`
	MatchExpressions MatchExpressions `yaml:"matchExpressions,omitempty"`
}

type Rules struct {
	Type           string           `yaml:"type"`
	LabelSelectors []LabelSelectors `yaml:"labelSelectors"`
}