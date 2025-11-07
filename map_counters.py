#!/usr/bin/env python3
"""
Reverse-engineer the mapping from Counters_f_*.raw files to CSV column names.

Strategy:
1. Parse CSV to get all counter names and values
2. Read each Counters_f_*.raw file as float32 array
3. For each file, compare its values against CSV columns
4. Find best match by correlation/exact match
5. Generate mapping table
"""

import csv
import struct
import os
import sys
from pathlib import Path

def read_counter_file(path):
    """Read a Counters_f_*.raw file as array of float32 values."""
    with open(path, 'rb') as f:
        data = f.read()
    # Each value is 4 bytes (float32)
    count = len(data) // 4
    values = struct.unpack(f'<{count}f', data)
    return list(values)

def parse_csv(csv_path):
    """Parse Xcode Counters CSV file.
    Returns: (counter_names, encoder_data)
    - counter_names: list of counter names (excluding first 5 metadata columns)
    - encoder_data: list of dicts, each containing counter values for one encoder
    """
    with open(csv_path, 'r') as f:
        reader = csv.reader(f)
        headers = next(reader)

        # Skip first 5 columns (Index, Encoder FunctionIndex, CommandBuffer Label, Encoder Label, empty)
        counter_names = headers[5:]

        encoder_data = []
        for row in reader:
            # Skip first 5 columns to get counter values
            values = []
            for val in row[5:]:
                try:
                    values.append(float(val))
                except (ValueError, IndexError):
                    values.append(0.0)
            encoder_data.append({
                'index': row[0] if len(row) > 0 else '',
                'encoder_func_idx': row[1] if len(row) > 1 else '',
                'cmd_buffer_label': row[2] if len(row) > 2 else '',
                'encoder_label': row[3] if len(row) > 3 else '',
                'values': values
            })

    return counter_names, encoder_data

def find_best_match(file_values, csv_columns, counter_names, tolerance=0.01):
    """
    Find which CSV column best matches the file values.

    Args:
        file_values: numpy array of float32 values from raw file
        csv_columns: list of numpy arrays, one per CSV column
        counter_names: list of counter names corresponding to csv_columns
        tolerance: relative tolerance for considering values equal

    Returns:
        (column_idx, counter_name, confidence) or None if no good match
    """
    best_match = None
    best_score = 0

    # For each encoder's data in the file
    for encoder_idx in range(min(len(file_values), len(csv_columns[0]))):
        file_val = file_values[encoder_idx]

        # Check each CSV column
        for col_idx, col_values in enumerate(csv_columns):
            if encoder_idx >= len(col_values):
                continue

            csv_val = col_values[encoder_idx]

            # Check for exact match or close match
            if abs(file_val - csv_val) < tolerance:
                # Found a match for this encoder
                if best_match is None or col_idx != best_match[0]:
                    # New column match
                    if best_match is None:
                        best_match = (col_idx, counter_names[col_idx], 1)
                    elif best_match[0] != col_idx:
                        # Multiple columns match - ambiguous
                        pass

    return best_match

def analyze_mapping(trace_dir, csv_path):
    """Analyze the mapping between raw files and CSV columns."""

    # Parse CSV
    counter_names, encoder_data = parse_csv(csv_path)
    print(f"CSV has {len(counter_names)} counter columns")
    print(f"CSV has {len(encoder_data)} encoder records")

    # Convert CSV data to column arrays
    csv_columns = []
    for col_idx in range(len(counter_names)):
        col_values = [enc['values'][col_idx] for enc in encoder_data]
        csv_columns.append(col_values)

    # Find all Counters_f_*.raw files
    raw_files = sorted(Path(trace_dir).glob("Counters_f_*.raw"))
    print(f"\nFound {len(raw_files)} Counters_f_*.raw files\n")

    # Analyze each file
    mapping = {}
    for raw_file in raw_files:
        file_idx = int(raw_file.stem.split('_')[-1])

        try:
            file_values = read_counter_file(raw_file)
            print(f"File {file_idx}: {len(file_values)} values")

            # Try to find matching CSV column
            # Strategy: The raw file contains one sample per time point
            # CSV only has aggregate values per encoder
            # We need to compare the first value (for first encoder)

            # Look at first value from file (should correspond to CSV encoder row 0)
            if len(file_values) > 0 and len(encoder_data) > 0:
                first_val = file_values[0]

                # Try to find a CSV column with matching value
                best_col_idx = None
                best_diff = float('inf')
                tolerance = 0.01  # 1% tolerance

                for col_idx, col_values in enumerate(csv_columns):
                    if len(col_values) > 0:
                        csv_val = col_values[0]

                        # Skip zero values (too common to be distinctive)
                        if abs(csv_val) < 0.001:
                            continue

                        # Calculate relative difference
                        if abs(csv_val) > 0:
                            rel_diff = abs((first_val - csv_val) / csv_val)
                        else:
                            rel_diff = abs(first_val - csv_val)

                        if rel_diff < tolerance and rel_diff < best_diff:
                            best_diff = rel_diff
                            best_col_idx = col_idx

                if best_col_idx is not None:
                    counter_name = counter_names[best_col_idx]
                    csv_val = csv_columns[best_col_idx][0]
                    mapping[file_idx] = {
                        'counter_name': counter_name,
                        'csv_column': best_col_idx,
                        'confidence': 'high' if best_diff < 0.001 else 'medium',
                        'file_val': first_val,
                        'csv_val': csv_val,
                        'rel_diff': best_diff
                    }
                    print(f"  → Matched to column {best_col_idx}: {counter_name}")
                    print(f"     File[0]: {first_val:.6f}, CSV: {csv_val:.6f}, diff: {best_diff*100:.2f}%")
                else:
                    print(f"  → No match found (first_val={first_val:.6f})")
                    mapping[file_idx] = None
            else:
                print(f"  → No data to match")
                mapping[file_idx] = None

        except Exception as e:
            print(f"  → Error: {e}")
            mapping[file_idx] = None

        print()

    return mapping, counter_names

def print_mapping_table(mapping, counter_names):
    """Print the mapping table in a readable format."""
    print("\n" + "="*80)
    print("COUNTER FILE TO CSV COLUMN MAPPING")
    print("="*80)
    print(f"{'File':<10} {'CSV Col':<10} {'Confidence':<12} {'Counter Name':<50}")
    print("-"*80)

    for file_idx in sorted(mapping.keys()):
        if mapping[file_idx] is not None:
            m = mapping[file_idx]
            print(f"{file_idx:<10} {m['csv_column']:<10} {m['confidence']:<12} {m['counter_name']:<50}")
        else:
            print(f"{file_idx:<10} {'N/A':<10} {'none':<12} {'(no match)':<50}")

    print("="*80)

def generate_go_code(mapping, counter_names):
    """Generate Go code with the mapping table."""
    print("\n// Auto-generated mapping from Counters_f_*.raw file index to counter name")
    print("var counterFileToName = map[int]string{")

    for file_idx in sorted(mapping.keys()):
        if mapping[file_idx] is not None:
            counter_name = mapping[file_idx]['counter_name']
            # Escape quotes in counter name
            counter_name = counter_name.replace('"', '\\"')
            print(f'\t{file_idx}: "{counter_name}",')

    print("}")

def main():
    if len(sys.argv) < 3:
        print("Usage: map_counters.py <trace_dir> <csv_path>")
        print()
        print("Example:")
        print("  map_counters.py testdata/traces/01-single-encoder/01-single-encoder-run1-perf.gputrace/01-single-encoder-run1.gputrace.gpuprofiler_raw \\")
        print("                  testdata/traces/01-single-encoder/01-single-encoder-run1\\ Counters.csv")
        sys.exit(1)

    trace_dir = sys.argv[1]
    csv_path = sys.argv[2]

    if not os.path.isdir(trace_dir):
        print(f"Error: {trace_dir} is not a directory")
        sys.exit(1)

    if not os.path.isfile(csv_path):
        print(f"Error: {csv_path} is not a file")
        sys.exit(1)

    mapping, counter_names = analyze_mapping(trace_dir, csv_path)
    print_mapping_table(mapping, counter_names)
    generate_go_code(mapping, counter_names)

if __name__ == '__main__':
    main()
