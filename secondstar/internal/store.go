package internal

import (
	"sync"
)

// KeyValueStore represents an in-memory key-value store.
type KeyValueStore struct {
	data  map[string]*State
	mutex sync.RWMutex
}

// NewKeyValueStore creates a new instance of KeyValueStore.
func NewKeyValueStore() *KeyValueStore {
	return &KeyValueStore{
		data:  make(map[string]*State),
		mutex: sync.RWMutex{},
	}
}

// Get retrieves a value from the store by key.
func (s *KeyValueStore) Get(key string) (*State, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	val, ok := s.data[key]
	return val, ok
}

// Set adds or updates a key-value pair in the store.
func (s *KeyValueStore) Set(key string, value *State) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.data[key] = value
}

// Delete removes a key-value pair from the store.
func (s *KeyValueStore) Delete(key string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.data, key)
}

// List retrieves all key-value pairs from the store.
func (s *KeyValueStore) List() map[string]*State {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	// Create a copy to avoid race conditions
	copyMap := make(map[string]*State)
	for key, value := range s.data {
		copyMap[key] = value
	}
	return copyMap
}
