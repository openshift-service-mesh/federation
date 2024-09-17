package fds

import (
	"sync"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
)

// ImportedServiceStore is a thread-safe wrapper for current state of imported services
type ImportedServiceStore struct {
	mu               sync.RWMutex
	importedServices []*v1alpha1.ExportedService
}

func NewImportedServiceStore() *ImportedServiceStore {
	return &ImportedServiceStore{}
}

func (s *ImportedServiceStore) Update(importedServices []*v1alpha1.ExportedService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	newImportedServices := make([]*v1alpha1.ExportedService, 0, len(importedServices))
	for _, svc := range importedServices {
		newImportedServices = append(newImportedServices, svc.DeepCopy())
	}
	s.importedServices = newImportedServices
}

// GetAll returns copy of the current state
func (s *ImportedServiceStore) GetAll() []*v1alpha1.ExportedService {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*v1alpha1.ExportedService, 0, len(s.importedServices))
	for _, svc := range s.importedServices {
		out = append(out, svc.DeepCopy())
	}
	return out
}
