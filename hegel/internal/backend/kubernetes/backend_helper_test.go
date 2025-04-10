package kubernetes

// NewTestBackend isn't representative of how Backends are constructed but is useful
// when wanting to validate the business logic around data retrieval and conversion.
func NewTestBackend(c listerClient, closer <-chan struct{}) *Backend {
	return &Backend{
		client: c,
		closer: closer,
	}
}
