package clone

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"
)

// CopyConstructor returns a type-safe copy function for type T
func CopyConstructor[T any]() func(T) T {
	var zero T
	protoType := reflect.TypeOf(zero)

	return func(src T) T {
		srcValue := reflect.ValueOf(src)
		if srcValue.Type() != protoType {
			panic(fmt.Sprintf("type mismatch: expected %v, got %v",
				protoType, srcValue.Type()))
		}

		copied := deepCopy(srcValue)
		return copied.Interface().(T)
	}
}

// Types that should not be copied and need special handling
var nonCopyableTypes = map[reflect.Type]bool{
	reflect.TypeOf(sync.Mutex{}):     true,
	reflect.TypeOf(sync.RWMutex{}):   true,
	reflect.TypeOf(sync.WaitGroup{}): true,
	reflect.TypeOf(sync.Once{}):      true,
	reflect.TypeOf(sync.Cond{}):      true,
}

func isNonCopyableType(t reflect.Type) bool {
	return nonCopyableTypes[t]
}

func deepCopy(src reflect.Value) reflect.Value {
	if !src.IsValid() {
		return reflect.Value{}
	}

	// Check for non-copyable types first
	if isNonCopyableType(src.Type()) {
		return reflect.Zero(src.Type())
	}

	switch src.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32,
		reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return src

	case reflect.Ptr:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.New(src.Type().Elem())
		dst.Elem().Set(deepCopy(src.Elem()))
		return dst

	case reflect.Slice:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.MakeSlice(src.Type(), src.Len(), src.Cap())
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(deepCopy(src.Index(i)))
		}
		return dst

	case reflect.Array:
		dst := reflect.New(src.Type()).Elem()
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(deepCopy(src.Index(i)))
		}
		return dst

	case reflect.Map:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.MakeMap(src.Type())
		for _, key := range src.MapKeys() {
			dst.SetMapIndex(deepCopy(key), deepCopy(src.MapIndex(key)))
		}
		return dst

	case reflect.Struct:
		return copyStruct(src)

	case reflect.Interface:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		return deepCopy(src.Elem())

	case reflect.Chan:
		return reflect.MakeChan(src.Type(), 0)

	case reflect.Func:
		return src

	default:
		return reflect.Zero(src.Type())
	}
}

func copyStruct(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()

	// First, copy the entire struct memory (this handles all primitive fields)
	if src.CanAddr() {
		copyStructMemory(dst, src)
	} else {
		// If src is not addressable, create an addressable copy first
		temp := reflect.New(src.Type()).Elem()
		temp.Set(src)
		copyStructMemory(dst, temp)
	}

	// Then, fix up fields that need special handling
	for i := 0; i < src.NumField(); i++ {
		field := src.Field(i)
		fieldType := field.Type()

		// Handle non-copyable types
		if isNonCopyableType(fieldType) {
			if dst.Field(i).CanSet() {
				dst.Field(i).Set(reflect.Zero(fieldType))
			} else {
				// Zero out unexported non-copyable field
				zeroUnexportedField(dst.Field(i))
			}
			continue
		}

		// Handle pointer, slice, map, interface fields that need deep copying
		if needsDeepCopy(fieldType) {
			if dst.Field(i).CanSet() {
				dst.Field(i).Set(deepCopy(field))
			} else {
				// Handle unexported field that needs deep copying
				deepCopyUnexportedField(dst.Field(i), field)
			}
		}

		// Primitive types and structs are already handled by memory copy
	}

	return dst
}

func needsDeepCopy(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface:
		return true
	case reflect.Struct:
		// Check if struct contains fields that need deep copying
		for i := 0; i < t.NumField(); i++ {
			if needsDeepCopy(t.Field(i).Type) {
				return true
			}
		}
		return false
	case reflect.Array:
		return needsDeepCopy(t.Elem())
	default:
		return false
	}
}

func copyStructMemory(dst, src reflect.Value) {
	if dst.CanAddr() && src.CanAddr() {
		dstPtr := unsafe.Pointer(dst.UnsafeAddr())
		srcPtr := unsafe.Pointer(src.UnsafeAddr())
		size := dst.Type().Size()
		copy((*[1024]byte)(dstPtr)[:size], (*[1024]byte)(srcPtr)[:size])
	}
}

func zeroUnexportedField(field reflect.Value) {
	if field.CanAddr() {
		ptr := unsafe.Pointer(field.UnsafeAddr())
		size := field.Type().Size()
		for i := uintptr(0); i < size; i++ {
			*(*byte)(unsafe.Pointer(uintptr(ptr) + i)) = 0
		}
	}
}

func deepCopyUnexportedField(dst, src reflect.Value) {
	if !dst.CanAddr() {
		return
	}

	switch src.Kind() {
	case reflect.Ptr:
		if src.IsNil() {
			// Already zeroed by memory copy
			return
		}
		// Create new pointer
		newPtr := reflect.New(src.Type().Elem())
		if src.Elem().CanInterface() {
			newPtr.Elem().Set(deepCopy(src.Elem()))
		} else {
			deepCopyUnexportedField(newPtr.Elem(), src.Elem())
		}
		*(*unsafe.Pointer)(unsafe.Pointer(dst.UnsafeAddr())) = unsafe.Pointer(newPtr.Pointer())

	case reflect.Slice:
		if src.IsNil() {
			return
		}
		newSlice := reflect.MakeSlice(src.Type(), src.Len(), src.Cap())
		for i := 0; i < src.Len(); i++ {
			if src.Index(i).CanSet() {
				newSlice.Index(i).Set(deepCopy(src.Index(i)))
			} else {
				deepCopyUnexportedField(newSlice.Index(i), src.Index(i))
			}
		}
		// Copy slice header
		sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(dst.UnsafeAddr()))
		newSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(newSlice.UnsafeAddr()))
		*sliceHeader = *newSliceHeader

	case reflect.Map:
		if src.IsNil() {
			return
		}
		newMap := reflect.MakeMap(src.Type())
		for _, key := range src.MapKeys() {
			newMap.SetMapIndex(deepCopy(key), deepCopy(src.MapIndex(key)))
		}
		*(*unsafe.Pointer)(unsafe.Pointer(dst.UnsafeAddr())) = unsafe.Pointer(newMap.Pointer())

	case reflect.Struct:
		// Recursively handle struct
		temp := copyStruct(src)
		copyStructMemory(dst, temp)
	}
}
