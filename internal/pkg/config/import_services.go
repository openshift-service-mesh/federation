package config

import (
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
)

type ImportedServiceSet struct {
	Rules []Rules `yaml:"rules"`
}

type ImportedService struct {
	AppName         string
	ServiceHostname string
	ServicePorts    []*istionetv1alpha3.ServicePort
	Endpoints       []Remote
}
