/*
Package staticroute provides tools for building EC2 Instance Metadata static routes from
the set of data endpoints. A data endpoint is an one that serves instance specific data.
*/
package staticroute

import (
	"sort"
	"strings"
)

// Builder constructs a set of Route objects. Endpoints added via FromEndpoint will be result
// in a static route for each level of endpoint nesting. The root route is always an empty string.
// Endpoints that are descendable will be appended with a slash. For example, adding the endpoint
// "/foo/bar/baz" will result in the following routes:
//
//	"/foo/bar" -> baz
//	"/foo" -> bar/
//	"" -> foo/
type Builder map[string]unorderedSet

// NewBuilder returns a new Builder instance.
func NewBuilder() Builder {
	return make(map[string]unorderedSet)
}

// FromEndpoint adds endpoint to b. endpoint should be of URL path form such as "/foo/bar".
// FromEndpoint can be called multiple times.
func (b Builder) FromEndpoint(endpoint string) {
	// Ensure our endpoint begins with a `/` so we can add to the root route for endpoint.
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	// Split the endpoint into its components so we can build the pieces we need.
	split := strings.Split(endpoint, "/")

	// Iterate over the components in reverse order so we can build parent paths for every
	// level of path nesting and track the child part.
	for i := len(split) - 1; i > 0; i-- {
		concat := strings.Join(split[:i], "/")
		if _, ok := b[concat]; !ok {
			b[concat] = newUnorderedSet()
		}
		b[concat].Insert(split[i])
	}
}

// Build returns a slice of Route objects containing an Endpoint and its associated child
// elements for the response body. The root route is identified by an empty string for the
// Endpoint field of Route.
func (b Builder) Build() []Route {
	var routes sortableRoutes

	for parent, children := range b {
		r := Route{Endpoint: parent}

		// Add children to the route prepending a slash for any child that is also a parent.
		children.Range(func(child string) {
			asParent := strings.Join([]string{parent, child}, "/")

			// If the child is also a parent, append a slash so the consumer knows it is a
			// descendable directory.
			if _, ok := b[asParent]; ok {
				child += "/"
			}

			r.Children = append(r.Children, child)
		})

		sort.Strings(r.Children)

		routes = append(routes, r)
	}

	// Sort for determinism, no other reason.
	sort.Sort(routes)

	return routes
}

// Route is an endpoint and its associated child elements.
type Route struct {
	Endpoint string
	Children []string
}
