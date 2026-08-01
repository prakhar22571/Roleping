package adapters

import (
	"fmt"

	"github.com/syumai/workers/cloudflare/fetch"
)

type Registry struct {
	adapters map[AdapterType]Adapter
}

func NewRegistry(client *fetch.Client) *Registry {
	return &Registry{
		adapters: map[AdapterType]Adapter{
			Amazon:     NewAmazonAdapter(client),
			Greenhouse: NewGreenhouseAdapter(client),
			Lever:      NewLeverAdapter(client),
		},
	}
}

func (r *Registry) Get(t AdapterType) (Adapter, error) {
	adapter, ok := r.adapters[t]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for type: %s", t)
	}
	return adapter, nil
}
