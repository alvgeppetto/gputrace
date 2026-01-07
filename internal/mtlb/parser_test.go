package mtlb

import (
	"encoding/binary"
	"testing"
)

func TestParseMTLB(t *testing.T) {
	// Create a dummy MTLB file
	// Header is 48 bytes
	data := make([]byte, 150)

	copy(data[0:4], []byte("MTLB"))
	binary.LittleEndian.PutUint32(data[4:8], 1)     // Version
	binary.LittleEndian.PutUint64(data[16:24], 150) // TotalSize

	// Function table at offset 48 (where ListFunctions starts scanning)
	binary.LittleEndian.PutUint64(data[24:32], 48) // FunctionTable
	binary.LittleEndian.PutUint64(data[32:40], 48) // StringTable
	binary.LittleEndian.PutUint64(data[40:48], 120) // BytecodeOffset

	// Add function entries with NAMED tags (ListFunctions looks for these)
	// Format: "NAMED\x00" + function_name + "\x00"
	offset := 48
	copy(data[offset:], []byte("NAMED\x00function1\x00"))
	offset += 6 + 10 // "NAMED\x00" (6) + "function1\x00" (10)
	copy(data[offset:], []byte("NAMED\x00function2\x00"))

	lib, err := ParseMTLB(data)
	if err != nil {
		t.Fatalf("ParseMTLB failed: %v", err)
	}

	if lib.Header.Version != 1 {
		t.Errorf("Expected version 1, got %d", lib.Header.Version)
	}

	funcs, err := lib.ListFunctions()
	if err != nil {
		t.Fatalf("ListFunctions failed: %v", err)
	}

	if len(funcs) != 2 {
		t.Errorf("Expected 2 functions, got %d", len(funcs))
	}

	if len(funcs) > 0 && funcs[0] != "function1" {
		t.Errorf("Expected function1, got %s", funcs[0])
	}
	if len(funcs) > 1 && funcs[1] != "function2" {
		t.Errorf("Expected function2, got %s", funcs[1])
	}
}
