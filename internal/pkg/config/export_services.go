package config

type ExportedServices struct {
	// TODO: remove rules as we will not support other types than LabelSelector in the near future
	Rules []Rules `yaml:"rules"`
}
