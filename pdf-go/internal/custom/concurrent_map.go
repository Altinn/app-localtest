package custom

import "sync"

// concurrentMap wraps a map with a mutex for safe concurrent access.
type concurrentMap[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]V
}

func newConcurrentMap[K comparable, V any]() *concurrentMap[K, V] {
	return &concurrentMap[K, V]{m: make(map[K]V)}
}

func (cm *concurrentMap[K, V]) set(k K, v V) {
	cm.mu.Lock()
	cm.m[k] = v
	cm.mu.Unlock()
}

func (cm *concurrentMap[K, V]) getAndDelete(k K) (V, bool) {
	cm.mu.Lock()
	v, ok := cm.m[k]
	if ok {
		delete(cm.m, k)
	}
	cm.mu.Unlock()
	return v, ok
}

func (cm *concurrentMap[K, V]) drain(consume func(V)) {
	cm.mu.Lock()
	old := cm.m
	cm.m = make(map[K]V)
	cm.mu.Unlock()
	for _, v := range old {
		consume(v)
	}
}
