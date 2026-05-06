package lib

import "reflect"

// isNil is a robust nil check for interfaces including typed nil pointers.
func isNil(i interface{}) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Ptr {
		return v.IsNil()
	}
	return false
}
