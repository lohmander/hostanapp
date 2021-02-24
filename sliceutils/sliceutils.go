package sliceutils

import (
	"reflect"

	"github.com/thoas/go-funk"
)

type ToString interface {
	ToString() string
}

func Includes(x interface{}, y ToString) bool {
	return funk.Find(x, func(z ToString) bool {
		return y.ToString() == z.ToString()
	}) != nil
}

func Subtract(x, y interface{}) interface{} {
	xVal := reflect.ValueOf(x)
	out := reflect.MakeSlice(reflect.SliceOf(xVal.Type().Elem()), 0, 0)

	for i := 0; i < xVal.Len(); i++ {
		val, ok := xVal.Index(i).Interface().(ToString)

		if ok && !Includes(y, val) {
			out = reflect.Append(out, xVal.Index(i))
		}
	}

	return out.Interface()
}

func Intersection(x, y interface{}) interface{} {
	xVal := reflect.ValueOf(x)
	out := reflect.MakeSlice(reflect.SliceOf(xVal.Type().Elem()), 0, 0)

	for i := 0; i < xVal.Len(); i++ {
		val, ok := xVal.Index(i).Interface().(ToString)

		if ok && Includes(y, val) {
			out = reflect.Append(out, xVal.Index(i))
		}
	}

	return out.Interface()
}
