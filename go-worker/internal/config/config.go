package config

import (
	"database/sql"

	"github.com/syumai/workers/cloudflare"
	"github.com/syumai/workers/cloudflare/fetch"
	"github.com/syumai/workers/cloudflare/kv"
)

const (
	DefaultModelA = "nvidia/nemotron-3-ultra-550b-a55b:free"
	DefaultModelB = "inclusionai/ling-3.0-flash:free"
)

// Env bundles the bindings and vars a request/task needs. It is constructed
// fresh per request since the underlying cloudflare bindings are tied to the
// current runtime context.
type Env struct {
	DB               *sql.DB
	ConfigKV         *kv.Namespace
	OpenRouterAPIKey string
	ResendAPIKey     string
	AlertToEmail     string
	AlertFromEmail   string
}

func Load() (*Env, error) {
	db, err := sql.Open("d1", "DB")
	if err != nil {
		return nil, err
	}

	configKV, err := kv.NewNamespace("CONFIG_KV")
	if err != nil {
		return nil, err
	}

	return &Env{
		DB:               db,
		ConfigKV:         configKV,
		OpenRouterAPIKey: cloudflare.Getenv("OPENROUTER_API_KEY"),
		ResendAPIKey:     cloudflare.Getenv("RESEND_API_KEY"),
		AlertToEmail:     cloudflare.Getenv("ALERT_TO_EMAIL"),
		AlertFromEmail:   cloudflare.Getenv("ALERT_FROM_EMAIL"),
	}, nil
}

// NewFetchClient returns a *fetch.Client backed by the Workers fetch()
// binding. A plain &http.Client{} routes through Go's standard "net" package,
// which depends on a "netdev" network device abstraction that isn't
// available in the Workers WASM runtime and fails with "Netdev not set" (and
// can hang the whole request rather than erroring cleanly). This must be
// used for every outbound request - adapters, OpenRouter, Resend.
func NewFetchClient() *fetch.Client {
	return fetch.NewClient()
}

// jsNullString is what syscall/js's Value.String() returns for a JS null or
// undefined value (a documented quirk, not an error) - KV.GetString surfaces
// this literal string rather than an empty one when a key doesn't exist.
const jsNullString = "<null>"

func isUnset(value string, err error) bool {
	return err != nil || value == "" || value == jsNullString
}

// ModelIDs reads the hot-swappable classification model config from KV,
// falling back to the built-in defaults when a key hasn't been set.
func (e *Env) ModelIDs() (modelA string, modelB string) {
	modelA, err := e.ConfigKV.GetString("model_a_id", nil)
	if isUnset(modelA, err) {
		modelA = DefaultModelA
	}
	modelB, err = e.ConfigKV.GetString("model_b_id", nil)
	if isUnset(modelB, err) {
		modelB = DefaultModelB
	}
	return modelA, modelB
}
