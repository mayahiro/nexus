package browser

import (
	"fmt"
	"sync"

	"github.com/mayahiro/nexus/internal/target/browser/chromium"
	"github.com/mayahiro/nexus/internal/target/browser/lightpanda"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

var backendFactoriesMu sync.RWMutex
var backendFactories = map[spec.BackendName]func() spec.Backend{
	spec.BackendChromium: func() spec.Backend { return chromium.New() },
	spec.BackendLightpanda: func() spec.Backend {
		return lightpanda.New()
	},
}

func NewBackend(name spec.BackendName) (spec.Backend, error) {
	backendFactoriesMu.RLock()
	factory, ok := backendFactories[name]
	backendFactoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown browser backend: %s", name)
	}

	return factory(), nil
}

func SetBackendFactory(name spec.BackendName, factory func() spec.Backend) func() {
	backendFactoriesMu.Lock()
	previous, hadPrevious := backendFactories[name]
	if factory == nil {
		delete(backendFactories, name)
	} else {
		backendFactories[name] = factory
	}
	backendFactoriesMu.Unlock()

	return func() {
		backendFactoriesMu.Lock()
		defer backendFactoriesMu.Unlock()
		if hadPrevious {
			backendFactories[name] = previous
			return
		}
		delete(backendFactories, name)
	}
}
