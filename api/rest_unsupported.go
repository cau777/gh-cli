package api

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotSupported indicates that the requested operation, flag, or JSON field
// cannot be fulfilled using the GitHub REST API.
var ErrNotSupported = errors.New("not all features are supported")

// NotSupportedError returns an error wrapping ErrNotSupported with a specific reason.
func NotSupportedError(reason string) error {
	if reason == "" {
		return ErrNotSupported
	}
	return fmt.Errorf("%w: %s", ErrNotSupported, reason)
}

// ValidateSupportedFields checks whether any requested JSON fields are present
// in unsupported and returns a NotSupportedError listing them if so.
func ValidateSupportedFields(requested []string, unsupported map[string]string) error {
	if len(requested) == 0 || len(unsupported) == 0 {
		return nil
	}

	var found []string
	var reasons []string
	for _, f := range requested {
		if reason, ok := unsupported[f]; ok {
			found = append(found, f)
			if reason != "" {
				reasons = append(reasons, fmt.Sprintf("%s (%s)", f, reason))
			} else {
				reasons = append(reasons, f)
			}
		}
	}
	if len(found) == 0 {
		return nil
	}

	return NotSupportedError(fmt.Sprintf("requested JSON field(s) not available via REST: %s", strings.Join(reasons, ", ")))
}
