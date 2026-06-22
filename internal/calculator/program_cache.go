package calculator

import (
	"sync"

	"github.com/c3xdev/c3x/internal/expr"
)

// programCache memoises compiled expressions across resources. Each
// catalog expression compiles once; subsequent evaluations only run
// the VM. Cache keys include the dimension context ("qty:aws_instance.compute_hours")
// so two expressions with the same source text from different dimensions
// don't collide on lookup but also don't recompile when revisited.
//
// Concurrency: the cache is safe for read-heavy access under a mutex,
// so resource evaluation can fan out across goroutines safely.
type programCache struct {
	mu       sync.RWMutex
	programs map[string]expr.Program
}

func newProgramCache() *programCache {
	return &programCache{programs: map[string]expr.Program{}}
}

func (c *programCache) compile(key, source string) (expr.Program, error) {
	c.mu.RLock()
	if p, ok := c.programs[key]; ok {
		c.mu.RUnlock()
		return p, nil
	}
	c.mu.RUnlock()

	p, err := expr.Compile(source)
	if err != nil {
		return expr.Program{}, err
	}
	c.mu.Lock()
	c.programs[key] = p
	c.mu.Unlock()
	return p, nil
}
