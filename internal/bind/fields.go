package bind

import (
	"fmt"
	"reflect"
	"strings"
)

type source int

const (
	srcQuery source = iota
	srcHeader
	srcCookie
	srcPath
	srcJSON
	srcFile
	srcForm
)

// fieldInfo describes a bindable struct field.
type fieldInfo struct {
	src      source
	tagValue string
	fieldIdx []int
	value    reflect.Value
}

// buildFieldInfoSlice analyzes a struct type and returns a slice of fieldInfo.
func buildFieldInfoSlice(t reflect.Type) ([]fieldInfo, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w got: %q", ErrExpectedStruct, t.Kind())
	}

	fields := make([]fieldInfo, 0, t.NumField())

	if err := collectFields(t, nil, &fields); err != nil {
		return nil, err
	}

	if len(fields) == 0 {
		return nil, fmt.Errorf("bind: no bindable fields found in %q", t.Name())
	}

	return fields, nil
}

// collectFields recursively collects field information from a struct type,
// including embedded structs. The indexPath tracks the path to nested fields.
func collectFields(t reflect.Type, indexPath []int, fields *[]fieldInfo) error {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)

		if sf.PkgPath != "" && !sf.Anonymous {
			continue
		}

		if sf.Anonymous {
			fieldType := sf.Type
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}

			if fieldType.Kind() == reflect.Struct {
				embeddedPath := append(indexPath, i)
				if err := collectFields(fieldType, embeddedPath, fields); err != nil {
					return err
				}
				continue
			}
		}

		var (
			tagSrc   source
			tagValue string
			tagCnt   int
		)

		if v, ok := sf.Tag.Lookup("query"); ok {
			tagSrc = srcQuery
			tagValue = v
			tagCnt++
		}

		if v, ok := sf.Tag.Lookup("header"); ok {
			tagSrc = srcHeader
			tagValue = v
			tagCnt++
		}

		if v, ok := sf.Tag.Lookup("cookie"); ok {
			tagSrc = srcCookie
			tagValue = v
			tagCnt++
		}

		if v, ok := sf.Tag.Lookup("path"); ok {
			tagSrc = srcPath
			tagValue = v
			tagCnt++
		}

		if v, ok := sf.Tag.Lookup("json"); ok {
			tagSrc = srcJSON
			tagValue = strings.Split(v, ",")[0]
			tagCnt++
		}

		if v, ok := sf.Tag.Lookup("file"); ok {
			tagSrc = srcFile
			tagValue = v
			tagCnt++
		}

		if v, ok := sf.Tag.Lookup("form"); ok {
			tagSrc = srcForm
			tagValue = v
			tagCnt++
		}

		if tagCnt == 0 {
			continue
		}
		if tagCnt > 1 {
			return fmt.Errorf("%w: %s", ErrMultipleTags, sf.Name)
		}
		if tagValue == "" && tagSrc != srcJSON {
			return fmt.Errorf("%w: %s", ErrEmptyTag, sf.Name)
		}

		fullPath := append(append([]int{}, indexPath...), i)

		*fields = append(*fields, fieldInfo{
			src:      tagSrc,
			tagValue: tagValue,
			fieldIdx: fullPath,
		})
	}

	return nil
}

func getFieldByPath(v reflect.Value, fieldIdx []int) reflect.Value {
	current := v
	for _, i := range fieldIdx {
		current = current.Field(i)
	}
	return current
}
