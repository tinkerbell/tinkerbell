package constant

// IsDisabled checks whether the master disabled annotation is present on a resource.
// Returns true and the reason if the annotation exists, false and empty string otherwise.
func IsDisabled(annotations map[string]string) (bool, string) {
	reason, ok := annotations[DisabledAnnotation]
	return ok, reason
}
