package staticroute

type sortableRoutes []Route

func (r sortableRoutes) Len() int      { return len(r) }
func (r sortableRoutes) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r sortableRoutes) Less(i, j int) bool {
	return r[i].Endpoint < r[j].Endpoint
}
