package orderedmap

import (
	"encoding/json"
)

// Map is a simplified ordered map that uses a regular map for now to avoid external dependencies.
// In a real scenario, we would use a proper ordered map implementation.
type Map[K comparable, V any] struct {
	m map[K]V
}

func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		m: make(map[K]V),
	}
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	if m == nil || m.m == nil {
		var zero V
		return zero, false
	}
	v, ok := m.m[key]
	return v, ok
}

func (m *Map[K, V]) Set(key K, value V) {
	if m == nil {
		return
	}
	if m.m == nil {
		m.m = make(map[K]V)
	}
	m.m[key] = value
}

func (m *Map[K, V]) Len() int {
	if m == nil || m.m == nil {
		return 0
	}
	return len(m.m)
}

func (m *Map[K, V]) MarshalJSON() ([]byte, error) {
	if m == nil || m.m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m.m)
}

func (m *Map[K, V]) UnmarshalJSON(data []byte) error {
	m.m = make(map[K]V)
	return json.Unmarshal(data, &m.m)
}
