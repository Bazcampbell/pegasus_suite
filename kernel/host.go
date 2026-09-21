// kernel/host.go

package kernel

import (
	"encoding/json"
	"pegasus_suite/engine"
)

type host struct{ k *Kernel }

func (h *host) Settings(name string) (json.RawMessage, error) {
	return h.k.store.AppSettings(name)
}

func (h *host) Engine() *engine.Engine {
	return h.k.eng
}
