package bind

import (
	"reflect"
	"sync"
)

// fieldInfoCache maps a concrete struct type to a slice of fieldInfo.
var fieldInfoCache sync.Map

func cachedFieldInfo(t reflect.Type) ([]fieldInfo, error) {
	if v, ok := fieldInfoCache.Load(t); ok {
		return v.([]fieldInfo), nil
	}

	infos, err := buildFieldInfoSlice(t)
	if err != nil {
		return nil, err
	}

	fieldInfoCache.Store(t, infos)

	return infos, nil
}
