package provider

import "fmt"

type entry struct {
	id      ID
	name    string
	factory Factory
}

// Registry is built in main and passed down, so tests can use their own.
type Registry struct {
	entries []entry
}

func NewRegistry() *Registry {
	return &Registry{}
}

// Register panics on a duplicate ID because that is a wiring bug in main.
func (r *Registry) Register(id ID, name string, f Factory) {
	if _, ok := r.find(id); ok {
		panic(fmt.Sprintf("provider %q registered twice", id))
	}
	r.entries = append(r.entries, entry{id: id, name: name, factory: f})
}

func (r *Registry) Lookup(id ID) (Factory, bool) {
	e, ok := r.find(id)
	return e.factory, ok
}

// Name returns "" for an unknown ID.
func (r *Registry) Name(id ID) string {
	e, _ := r.find(id)
	return e.name
}

// IDs keeps the registration order, so Settings cycles in a fixed order.
func (r *Registry) IDs() []ID {
	if r == nil {
		return nil
	}
	ids := make([]ID, 0, len(r.entries))
	for _, e := range r.entries {
		ids = append(ids, e.id)
	}
	return ids
}

// Default is the first registered provider until config selects one.
func (r *Registry) Default() ID {
	if r == nil || len(r.entries) == 0 {
		return ""
	}
	return r.entries[0].id
}

func (r *Registry) find(id ID) (entry, bool) {
	if r == nil {
		return entry{}, false
	}
	for _, e := range r.entries {
		if e.id == id {
			return e, true
		}
	}
	return entry{}, false
}
