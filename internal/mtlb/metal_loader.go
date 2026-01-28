//go:build darwin

package mtlb

import (
	"fmt"

	"github.com/tmc/appledocs/generated/metal"
	"github.com/tmc/appledocs/generated/objc"
	"github.com/tmc/appledocs/generated/objectivec"
)

// MetalLibrary wraps a Metal library loaded via native APIs.
type MetalLibrary struct {
	library metal.MTLLibrary
	device  metal.MTLDevice
}

// LoadMTLBWithMetal loads an MTLB file using Metal's native APIs.
// This provides accurate function enumeration compared to binary parsing.
func LoadMTLBWithMetal(data []byte) (*MetalLibrary, error) {
	if len(data) < 4 || string(data[:4]) != "MTLB" {
		return nil, fmt.Errorf("not a valid MTLB file")
	}

	// Get the default Metal device
	devicePtr := metal.MTLCreateSystemDefaultDevice()
	if devicePtr == nil {
		return nil, fmt.Errorf("no Metal device available")
	}
	device := metal.NewMTLDeviceObject(objectivec.ObjectFrom(devicePtr))

	// Create NSData from bytes
	nsDataClass := objc.GetClass("NSData")
	nsData := objc.Send[objc.ID](objc.ID(uintptr(nsDataClass)), objc.Sel("dataWithBytes:length:"), &data[0], uint(len(data)))
	if nsData == 0 {
		return nil, fmt.Errorf("failed to create NSData")
	}

	// Create library from data
	var libErr objc.ID
	libraryID := objc.Send[objc.ID](device.GetID(), objc.Sel("newLibraryWithData:error:"), nsData, &libErr)
	if libraryID == 0 {
		errStr := "unknown error"
		if libErr != 0 {
			desc := objc.Send[objc.ID](libErr, objc.Sel("localizedDescription"))
			if desc != 0 {
				cstr := objc.Send[*byte](desc, objc.Sel("UTF8String"))
				errStr = objc.GoString(cstr)
			}
		}
		return nil, fmt.Errorf("failed to load library: %s", errStr)
	}

	library := metal.NewMTLLibraryObject(objectivec.ObjectFromID(libraryID))
	return &MetalLibrary{
		library: library,
		device:  device,
	}, nil
}

// FunctionNames returns all function names in the library.
func (ml *MetalLibrary) FunctionNames() []string {
	functionNamesID := objc.Send[objc.ID](ml.library.GetID(), objc.Sel("functionNames"))
	if functionNamesID == 0 {
		return nil
	}

	count := objc.Send[uint](functionNamesID, objc.Sel("count"))
	functions := make([]string, 0, count)
	for i := uint(0); i < count; i++ {
		nameID := objc.Send[objc.ID](functionNamesID, objc.Sel("objectAtIndex:"), i)
		if nameID != 0 {
			cstr := objc.Send[*byte](nameID, objc.Sel("UTF8String"))
			if cstr != nil {
				functions = append(functions, objc.GoString(cstr))
			}
		}
	}
	return functions
}

// FunctionCount returns the number of functions in the library.
func (ml *MetalLibrary) FunctionCount() int {
	functionNamesID := objc.Send[objc.ID](ml.library.GetID(), objc.Sel("functionNames"))
	if functionNamesID == 0 {
		return 0
	}
	return int(objc.Send[uint](functionNamesID, objc.Sel("count")))
}

// GetFunction returns a Metal function by name.
func (ml *MetalLibrary) GetFunction(name string) (metal.MTLFunction, error) {
	nsStringClass := objc.GetClass("NSString")
	nameStr := objc.Send[objc.ID](objc.ID(uintptr(nsStringClass)), objc.Sel("stringWithUTF8String:"), name+"\x00")

	fnID := objc.Send[objc.ID](ml.library.GetID(), objc.Sel("newFunctionWithName:"), nameStr)
	if fnID == 0 {
		return nil, fmt.Errorf("function not found: %s", name)
	}

	return metal.NewMTLFunctionObject(objectivec.ObjectFromID(fnID)), nil
}

// Label returns the library's label (if any).
func (ml *MetalLibrary) Label() string {
	labelID := objc.Send[objc.ID](ml.library.GetID(), objc.Sel("label"))
	if labelID == 0 {
		return ""
	}
	cstr := objc.Send[*byte](labelID, objc.Sel("UTF8String"))
	if cstr == nil {
		return ""
	}
	return objc.GoString(cstr)
}
