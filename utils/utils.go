package utils

import (
	"fmt"

	hostanv1alpha1 "github.com/lohmander/hostanapp/api/v1alpha1"
)

// StringSliceEquals compares two string slices
func StringSliceEquals(x1, x2 []string) bool {
	if len(x1) != len(x2) {
		return false
	}

	for i, part := range x1 {
		if part != x2[i] {
			return false
		}
	}

	return true
}

// ServiceWithNameInApp checks if there's a service with name in the app
func ServiceWithNameInApp(name string, app *hostanv1alpha1.App) bool {
	for _, service := range app.Spec.Services {
		if fmt.Sprintf("%s-%s", app.Name, service.Name) == name {
			return true
		}
	}

	return false
}

// UsesProviderWithNameInApp checks if there's a provider usage with name in the app
func UsesProviderWithNameInApp(name string, app *hostanv1alpha1.App) bool {
	for _, uses := range app.Spec.Uses {
		if fmt.Sprintf("%s-%s", app.Name, uses.Name) == name {
			return true
		}
	}

	return false
}
