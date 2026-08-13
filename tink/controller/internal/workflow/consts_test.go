package workflow

// String values whose occurrences are predominantly in tests. They live in the
// test package so production code stays free of test-driven constants; the few
// production occurrences remain string literals below goconst's threshold.
const (
	reasonComplete    = "Complete"
	reasonCreated     = "Created"
	messageJobCreated = "job created"
	valueTrue         = "true"
)
