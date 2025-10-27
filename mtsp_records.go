package gputrace

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// MTSP Record Types observed in capture files
const (
	RecordTypeCS     = "CS"     // Command submission with kernel name
	RecordTypeCt     = "Ct"     // Command type/transition?
	RecordTypeCU     = "CU"     // Command unknown?
	RecordTypeCulul  = "Culul"  // Command buffer marker
	RecordTypeCuw    = "Cuw"    // Command write?
	RecordTypeCi     = "Ci"     // Command info?
	RecordTypeCul    = "Cul"    // Command?
	RecordTypeCut    = "Cut"    // Command type extended?
)

// MTSPRecord represents a parsed MTSP record from the capture file.
type MTSPRecord struct {
	Type   string  // Record type (CS, CU, Culul, etc.)
	Offset int     // Offset in file where record starts
	Size   int     // Size of record in bytes
	Data   []byte  // Raw record data

	// Parsed fields (type-specific)
	Label      string   // For CS records: kernel/stream name
	Address    uint64   // Memory address
	Pointers   []uint64 // Referenced pointers
	Values     []uint32 // Embedded values
}

// ParseMTSPRecords parses records from the capture file.
func (t *Trace) ParseMTSPRecords() ([]MTSPRecord, error) {
	data := t.CaptureData

	// Skip MTSP header
	if len(data) < 16 {
		return nil, fmt.Errorf("capture data too small")
	}

	_, err := ReadMTSPHeader(data)
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Start parsing after header - records begin around offset 0x20 (32)
	// but we'll scan for them to be safe
	offset := 16
	var records []MTSPRecord

	for offset < len(data)-8 {
		// Read potential record size
		recordSize := int(binary.LittleEndian.Uint32(data[offset : offset+4]))

		// Validate size looks reasonable
		if recordSize == 0 || recordSize > 0x10000 || offset+recordSize > len(data) {
			offset += 4
			continue
		}

		// Extract potential record data
		recordData := data[offset : offset+recordSize]

		// Try to detect record type
		recordType := detectRecordType(recordData)

		// Only accept records with known types
		if recordType != "unknown" {
			record := MTSPRecord{
				Type:   recordType,
				Offset: offset,
				Size:   recordSize,
				Data:   recordData,
			}

			// Parse type-specific fields
			switch recordType {
			case RecordTypeCS:
				record.parseCSRecord()
			case RecordTypeCU, RecordTypeCut:
				record.parseCURecord()
			case RecordTypeCulul:
				record.parseCululRecord()
			}

			records = append(records, record)
			offset += recordSize
		} else {
			offset += 4
		}
	}

	return records, nil
}

// detectRecordType identifies the record type from its data.
func detectRecordType(data []byte) string {
	if len(data) < 16 {
		return "unknown"
	}

	// Check for known markers
	// Markers typically appear around offset 32 based on hex analysis
	for i := 8; i < min(len(data), 64); i++ {
		if i+5 <= len(data) && bytes.Equal(data[i:i+5], []byte("Culul")) {
			return RecordTypeCulul
		}
		if i+3 <= len(data) && bytes.Equal(data[i:i+3], []byte("Cuw")) {
			return RecordTypeCuw
		}
		if i+3 <= len(data) && bytes.Equal(data[i:i+3], []byte("Cut")) {
			return RecordTypeCut
		}
		// Check CS before Ct/Cul to avoid false matches
		if i+2 <= len(data) && bytes.Equal(data[i:i+2], []byte("CS")) {
			// Check that it's not part of another word
			if i+3 < len(data) && data[i+2] == 0 {
				return RecordTypeCS
			}
		}
		// Ct needs to be checked carefully to not match "Cut" or "Ctulul"
		if i+2 <= len(data) && bytes.Equal(data[i:i+2], []byte("Ct")) {
			if i+3 < len(data) && data[i+2] == 0 {
				return RecordTypeCt
			}
		}
		if i+3 <= len(data) && bytes.Equal(data[i:i+3], []byte("Cul")) {
			return RecordTypeCul
		}
		if i+2 <= len(data) && bytes.Equal(data[i:i+2], []byte("CU")) {
			if i+3 < len(data) && data[i+2] == 0 {
				return RecordTypeCU
			}
		}
		if i+2 <= len(data) && bytes.Equal(data[i:i+2], []byte("Ci")) {
			if i+3 < len(data) && data[i+2] == 0 {
				return RecordTypeCi
			}
		}
	}

	return "unknown"
}

// parseCSRecord parses a CS (Command Submission?) record.
// These often contain kernel/stream names.
func (r *MTSPRecord) parseCSRecord() {
	// CS records typically have format:
	// [size] [padding] [CS marker] [address] [string...]

	// Look for null-terminated string after CS marker
	for i := 0; i < len(r.Data)-4; i++ {
		if i+2 < len(r.Data) && r.Data[i] == 'C' && r.Data[i+1] == 'S' && r.Data[i+2] == 0 {
			// Found CS marker, look for string after address
			stringStart := i + 12 // Skip CS marker + padding + address
			if stringStart < len(r.Data) {
				if end := bytes.IndexByte(r.Data[stringStart:], 0); end != -1 {
					r.Label = string(r.Data[stringStart : stringStart+end])
				}
			}
			break
		}
	}
}

// parseCURecord parses a CU/Cut record.
// These may contain UUIDs or identifiers.
func (r *MTSPRecord) parseCURecord() {
	// Look for UUID-like strings (hexadecimal)
	for i := 0; i < len(r.Data)-16; i++ {
		// Check if we have a hex string
		if isHexString(r.Data[i : min(i+32, len(r.Data))]) {
			end := i
			for end < len(r.Data) && (isHex(r.Data[end]) || r.Data[end] == '-') {
				end++
			}
			if end > i {
				r.Label = string(r.Data[i:end])
				break
			}
		}
	}
}

// parseCululRecord parses a Culul (Command buffer) record.
func (r *MTSPRecord) parseCululRecord() {
	// Culul records mark command buffers
	// Format: [Culul marker] [padding] [address] [flags?]
	for i := 0; i < len(r.Data)-12; i++ {
		if i+5 <= len(r.Data) && bytes.Equal(r.Data[i:i+5], []byte("Culul")) {
			// Read address after marker
			if i+13 <= len(r.Data) {
				r.Address = binary.LittleEndian.Uint64(r.Data[i+5 : i+13])
			}
			break
		}
	}
}

// Helper functions

func isHexString(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	count := 0
	for i := 0; i < min(len(data), 32); i++ {
		if isHex(data[i]) {
			count++
		} else if data[i] == 0 {
			break
		} else {
			return false
		}
	}
	return count >= 8 // At least 8 hex chars
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f')
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// AnalyzeMTSPRecords provides a detailed analysis of MTSP records.
func (t *Trace) AnalyzeMTSPRecords() (string, error) {
	records, err := t.ParseMTSPRecords()
	if err != nil {
		return "", err
	}

	report := "=== MTSP Record Analysis ===\n\n"
	report += fmt.Sprintf("Total records: %d\n\n", len(records))

	// Count by type
	typeCounts := make(map[string]int)
	for _, record := range records {
		typeCounts[record.Type]++
	}

	report += "Record types:\n"
	for rtype, count := range typeCounts {
		report += fmt.Sprintf("  %-10s: %d records\n", rtype, count)
	}
	report += "\n"

	// Show first 20 records
	report += "First 20 records:\n"
	for i, record := range records {
		if i >= 20 {
			report += fmt.Sprintf("... and %d more\n", len(records)-20)
			break
		}

		info := fmt.Sprintf("  [%3d] offset=0x%06x size=%4d type=%-10s",
			i, record.Offset, record.Size, record.Type)

		if record.Label != "" {
			info += fmt.Sprintf(" label=%q", record.Label)
		}
		if record.Address != 0 {
			info += fmt.Sprintf(" addr=0x%x", record.Address)
		}

		report += info + "\n"
	}

	// Show CS records (kernel names)
	report += "\n=== CS Records (Kernel Names) ===\n"
	csCount := 0
	for _, record := range records {
		if record.Type == RecordTypeCS && record.Label != "" {
			if csCount < 30 {
				report += fmt.Sprintf("  %s\n", record.Label)
			}
			csCount++
		}
	}
	if csCount > 30 {
		report += fmt.Sprintf("... and %d more\n", csCount-30)
	}

	return report, nil
}
