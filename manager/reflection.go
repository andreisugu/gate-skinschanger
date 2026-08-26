package manager

import (
	"reflect"
	"unsafe"

	"go.minekube.com/gate/pkg/edition/java/profile"
)

// InjectPlayerProfileProperty injects or updates the "textures" property on a connected player's GameProfile in-place.
func InjectPlayerProfileProperty(p any, prop profile.Property) bool {
	if p == nil {
		return false
	}

	val := reflect.ValueOf(p)
	if val.Kind() == reflect.Ptr {
		elem := val.Elem()
		field := elem.FieldByName("profile")
		if field.IsValid() && field.CanAddr() {
			ptr := unsafe.Pointer(field.UnsafeAddr())
			profPtr := *(**profile.GameProfile)(ptr)
			if profPtr != nil {
				var newProps []profile.Property
				for _, existing := range profPtr.Properties {
					if existing.Name != "textures" {
						newProps = append(newProps, existing)
					}
				}
				if prop.Value != "" {
					newProps = append(newProps, prop)
				}
				profPtr.Properties = newProps
				return true
			}
		}
	}
	return false
}
