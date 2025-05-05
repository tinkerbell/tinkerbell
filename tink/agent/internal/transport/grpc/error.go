package grpc

type NoWorkflowError struct{}

func (e *NoWorkflowError) Error() string {
	return "No workflow found"
}

func (e *NoWorkflowError) NoWorkflow() bool {
	return true
}
