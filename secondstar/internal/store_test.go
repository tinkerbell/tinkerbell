package internal

import (
	"reflect"
	"sync"
	"testing"
)

func TestKeyValueStore_NewKeyValueStore(t *testing.T) {
	store := NewKeyValueStore()

	// Verify store is properly initialized
	if store == nil {
		t.Errorf("NewKeyValueStore returned nil")
	}

	// Verify map is empty
	if len(store.List()) != 0 {
		t.Errorf("Expected empty store, got %v", store.List())
	}

	// Verify mutex is properly initialized
	mutexValue := reflect.ValueOf(store).Elem().FieldByName("mutex")
	if !mutexValue.IsValid() {
		t.Errorf("RWMutex field not found")
	}
	if mutexValue.Kind() != reflect.Struct {
		t.Errorf("RWMutex field is not a struct")
	}
}

func TestKeyValueStore_SetAndGet(t *testing.T) {
	store := NewKeyValueStore()
	key := "test_key"
	value := &State{}

	// Set the value
	store.Set(key, value)

	// Get the value and verify
	retrieved, exists := store.Get(key)
	if !exists {
		t.Errorf("Value not found after setting")
	}
	if !reflect.DeepEqual(retrieved, value) {
		t.Errorf("Retrieved value doesn't match: expected %+v, got %+v",
			value, retrieved)
	}
}

func TestKeyValueStore_Delete(t *testing.T) {
	store := NewKeyValueStore()
	key := "test_key"
	value := &State{}

	// Set initial value
	store.Set(key, value)

	// Verify initial state
	retrieved, exists := store.Get(key)
	if !exists || !reflect.DeepEqual(retrieved, value) {
		t.Errorf("Initial value verification failed")
	}

	// Delete the value
	store.Delete(key)

	// Verify deletion
	retrieved, exists = store.Get(key)
	if exists {
		t.Errorf("Value found after deletion: %+v", retrieved)
	}
}

func TestKeyValueStore_List(t *testing.T) {
	store := NewKeyValueStore()
	values := map[string]*State{
		"key1": { /* State instance */ },
		"key2": { /* State instance */ },
		"key3": { /* State instance */ },
	}

	// Set multiple values
	for k, v := range values {
		store.Set(k, v)
	}

	// Get all values
	allValues := store.List()

	// Verify count
	if len(allValues) != len(values) {
		t.Errorf("Expected %d values, got %d", len(values), len(allValues))
	}

	// Verify each value
	for k, expectedValue := range values {
		actualValue, exists := allValues[k]
		if !exists {
			t.Errorf("Missing key: %s", k)
			continue
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Errorf("Mismatched value for key %s: expected %+v, got %+v",
				k, expectedValue, actualValue)
		}
	}
}

func TestKeyValueStore_ConcurrentAccess(t *testing.T) {
	store := NewKeyValueStore()
	key := "test_key"

	// Create multiple goroutines accessing the store
	var wgWriters sync.WaitGroup
	numGoroutines := 10

	// Writers
	for i := 0; i < numGoroutines; i++ {
		wgWriters.Add(1)
		go func(i int) {
			defer wgWriters.Done()
			newValue := &State{}
			store.Set(key, newValue)

			// Read back to verify
			retrieved, exists := store.Get(key)
			if !exists || !reflect.DeepEqual(retrieved, newValue) {
				t.Errorf("Concurrent read failed for goroutine %d", i)
			}
		}(i)
	}

	wgWriters.Wait()

	var wgReaders sync.WaitGroup
	// Readers
	for i := 0; i < numGoroutines; i++ {
		wgReaders.Add(1)
		go func() {
			defer wgReaders.Done()
			retrieved, exists := store.Get(key)
			if !exists || retrieved == nil {
				t.Errorf("Concurrent read failed")
			}
		}()
	}

	// Wait for all goroutines to finish
	wgReaders.Wait()
}

func TestKeyValueStore_MultipleKeys(t *testing.T) {
	store := NewKeyValueStore()
	values := map[string]*State{
		"key1": {},
		"key2": {},
		"key3": {},
	}

	// Set multiple values
	for k, v := range values {
		store.Set(k, v)
	}

	// Verify each value individually
	for k, expectedValue := range values {
		retrieved, exists := store.Get(k)
		if !exists {
			t.Errorf("Missing key: %s", k)
			continue
		}
		if !reflect.DeepEqual(retrieved, expectedValue) {
			t.Errorf("Mismatched value for key %s: expected %+v, got %+v",
				k, expectedValue, retrieved)
		}
	}
}

func TestKeyValueStore_NilValues(t *testing.T) {
	store := NewKeyValueStore()
	key := "test_key"

	// Try setting nil value
	store.Set(key, nil)

	// Get the value and verify
	retrieved, exists := store.Get(key)
	if !exists {
		t.Errorf("Nil value not stored")
	}
	if retrieved != nil {
		t.Errorf("Expected nil value, got %+v", retrieved)
	}
}
