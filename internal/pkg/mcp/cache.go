package mcp

import "sync"

type ServiceCache struct {
	sync.RWMutex
	data map[string]ServiceInfo
}

type ServiceInfo struct {
	Name      string
	Namespace string
}

func (c *ServiceCache) Update(key string, value ServiceInfo) {
	c.Lock()
	defer c.Unlock()
	c.data[key] = value
}

func (c *ServiceCache) Delete(key string) {
	c.Lock()
	defer c.Unlock()
	delete(c.data, key)
}

func (c *ServiceCache) List() []ServiceInfo {
	c.RLock()
	defer c.RUnlock()
	list := make([]ServiceInfo, 0, len(c.data))
	for _, value := range c.data {
		list = append(list, value)
	}
	return list
}

func NewServiceCache() *ServiceCache {
	return &ServiceCache{
		data: make(map[string]ServiceInfo),
	}
}
