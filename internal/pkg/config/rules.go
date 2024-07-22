package config

type MatchExpressions struct {
	Key      string   `yaml:"key"`
	Operator string   `yaml:"operator"`
	Values   []string `yaml:"values"`
}

type LabelSelectors struct {
	MatchLabels      map[string]string  `yaml:"matchLabels,omitempty"`
	MatchExpressions []MatchExpressions `yaml:"matchExpressions,omitempty"`
}

type Rules struct {
	Type           string           `yaml:"type"`
	LabelSelectors []LabelSelectors `yaml:"labelSelectors"`
}
