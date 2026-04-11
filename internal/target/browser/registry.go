package browser

import (
	"fmt"

	"github.com/mayahiro/nexus/internal/target/browser/chromium"
	"github.com/mayahiro/nexus/internal/target/browser/lightpanda"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

func NewBackend(name spec.BackendName) (spec.Backend, error) {
	switch name {
	case spec.BackendChromium:
		return chromium.New(), nil
	case spec.BackendLightpanda:
		return lightpanda.New(), nil
	default:
		return nil, fmt.Errorf("unknown browser backend: %s", name)
	}
}
