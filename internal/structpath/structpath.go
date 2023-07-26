package structpath

import (
	"fmt"
	"reflect"
	"strings"
)

func Lookup(target interface{}, path []string) (s string, ok bool) {
	currentValue := reflect.ValueOf(target)

	for _, pathElement := range path {
		pathElement := pathElement

		field := currentValue.FieldByNameFunc(func(fieldName string) bool {
			return strings.EqualFold(fieldName, pathElement)
		})

		if !field.IsValid() {
			return "", false
		}

		currentValue = field
	}

	result, ok := currentValue.Interface().(string)
	if ok {
		return result, true
	}

	stringerResult, ok := currentValue.Interface().(fmt.Stringer)
	if ok {
		return stringerResult.String(), true
	}

	return "", false
}
