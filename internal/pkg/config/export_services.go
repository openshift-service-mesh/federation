package config

type ExportedServiceSet struct {
	// TODO: remove rules as we will not support other types than LabelSelector in the near future
	Rules []Rules `yaml:"rules"`
}

// TODO: Refactor configuration structure
func (s *ExportedServiceSet) GetLabelSelectors() []LabelSelectors {
	if len(s.Rules) == 0 {
		return []LabelSelectors{}
	}
	return s.Rules[0].LabelSelectors
}
