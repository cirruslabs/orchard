package structpath

import (
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
	if !ok {
		return "", false
	}

	return result, true
}
