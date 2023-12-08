package runcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/astromechza/score-flyio/score"
)

var (
	placeholderRegEx = regexp.MustCompile(`\$(\$|{([a-zA-Z0-9.\-_\[\]"'#]+)})`)
)

// templatesContext ia an utility type that provides a context for '${...}' templates substitution
type templatesContext struct {
	meta               score.WorkloadSpecMetadata
	resourceProperties map[string]map[string]interface{}
}

// Substitute replaces all matching '${...}' templates in a source string
func (ctx *templatesContext) Substitute(src string) (string, error) {
	subErrors := make([]error, 0)
	output := placeholderRegEx.ReplaceAllStringFunc(src, func(str string) string {
		matches := placeholderRegEx.FindStringSubmatch(str)
		if len(matches) != 3 {
			subErrors = append(subErrors, fmt.Errorf("invalid substitution pattern '%s' - did you mean to escape it with $$", src))
			return src
		}
		// EDGE CASE: Captures "$$" sequences and empty templates "${}"
		if matches[2] == "" {
			return matches[1]
		}
		result, err := ctx.mapVar(matches[2])
		if err != nil {
			subErrors = append(subErrors, err)
		}
		return result
	})
	return output, errors.Join(subErrors...)
}

// MapVar replaces objects and properties references with corresponding values
// Returns an empty string if the reference can't be resolved
func (ctx *templatesContext) mapVar(ref string) (string, error) {
	if ref == "" || ref == "$" {
		return ref, nil
	}
	var segments = strings.SplitN(ref, ".", 3)
	switch segments[0] {
	case "metadata":
		if len(segments) == 2 {
			if val, exists := ctx.meta[segments[1]]; exists {
				switch typed := val.(type) {
				case string:
					return typed, nil
				default:
					vv, _ := json.Marshal(typed)
					return string(vv), nil
				}
			} else {
				return "", fmt.Errorf("expression '%s' refers to missing metadata key", ref)
			}
		}
	case "resources":
		if len(segments) == 3 {
			resource, ok := ctx.resourceProperties[segments[1]]
			if !ok {
				return "", fmt.Errorf("undefined resource '%s'", segments[1])
			}
			property, ok := resource[segments[2]]
			if !ok {
				return "", fmt.Errorf("property %s not set on resource type", segments[2])
			}
			return fmt.Sprint(property), nil
		}
	}
	return "", fmt.Errorf("unsupported expression reference '%s'", ref)
}
