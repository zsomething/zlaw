package push

// AdapterOwner is implemented by adapters that can deliver messages to the
// agent owner without an explicit target address. Not all adapters support
// this — for example, the CLI adapter returns ("", false).
type AdapterOwner interface {
	OwnerAddress() (address string, ok bool)
}
