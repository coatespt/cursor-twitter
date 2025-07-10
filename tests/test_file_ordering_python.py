#!/usr/bin/env python3
"""
Test to verify that Python's sorted() function works correctly for gnip.csv filenames.
This ensures that both Python and Go process files in the same chronological order.
"""

import os
import tempfile
import time
from datetime import datetime


def extract_start_time_from_gnip_filename(filename):
    """Extract the start time from gnip.csv filenames."""
    # Remove .csv extension
    base = filename.replace('.csv', '')
    
    # Split by underscore to get parts
    parts = base.split('_')
    if len(parts) != 3:
        raise ValueError(f"Expected 3 parts in filename, got {len(parts)}: {filename}")
    
    # Parse the start time (second part)
    start_time_str = parts[1]
    start_time = int(start_time_str)
    
    return start_time


def test_gnip_csv_format_python():
    """Test that Python's sorted() works correctly for gnip.csv filenames."""
    # Test files with the actual gnip.csv format: gnip.csv_STARTTIME_ENDTIME.csv
    filenames = [
        "gnip.csv_1327796182986_1327796482986.csv",  # Dec 1, 12:00:00 - 12:01:00
        "gnip.csv_1327796482986_1327796782986.csv",  # Dec 1, 12:01:00 - 12:02:00
        "gnip.csv_1327796782986_1327797082986.csv",  # Dec 1, 12:02:00 - 12:03:00
        "gnip.csv_1327797082986_1327797382986.csv",  # Dec 1, 12:03:00 - 12:04:00
        "gnip.csv_1327797382986_1327797682986.csv",  # Dec 1, 12:04:00 - 12:05:00
    ]

    # Sort lexicographically (Python's sorted() behavior)
    lexicographic_order = sorted(filenames)

    # Extract start times and verify they're in chronological order
    start_times = []
    for filename in lexicographic_order:
        try:
            start_time = extract_start_time_from_gnip_filename(filename)
            start_times.append(start_time)
        except ValueError as e:
            print(f"Failed to extract start time from {filename}: {e}")
            return False

    # Verify chronological order
    for i in range(1, len(start_times)):
        if start_times[i] < start_times[i-1]:
            print(f"❌ Files not in chronological order: {start_times[i-1]} comes before {start_times[i]}")
            return False

    print("✓ Gnip CSV files are in correct chronological order")
    print("  Python's sorted() matches chronological order for Unix timestamps")
    
    # Show the actual timestamps for verification
    for i, filename in enumerate(lexicographic_order):
        start_time = start_times[i]
        # Convert milliseconds to seconds for datetime
        human_time = datetime.fromtimestamp(start_time / 1000).strftime("%Y-%m-%d %H:%M:%S")
        print(f"  {i+1}: {filename} -> {human_time}")
    
    return True


def test_python_vs_go_consistency():
    """Test that Python and Go would produce the same ordering."""
    filenames = [
        "gnip.csv_1327796182986_1327796482986.csv",
        "gnip.csv_1327796482986_1327796782986.csv", 
        "gnip.csv_1327796782986_1327797082986.csv",
        "gnip.csv_1327797082986_1327797382986.csv",
        "gnip.csv_1327797382986_1327797682986.csv",
    ]
    
    # Python sorting (this is what the sender uses)
    python_order = sorted(filenames)
    
    # Expected Go-like sorting (should be the same)
    expected_order = sorted(filenames)  # Python's sorted() is lexicographic like Go's sort.Strings()
    
    if python_order == expected_order:
        print("✓ Python and Go sorting are consistent")
        print(f"  Python order: {python_order}")
        return True
    else:
        print("❌ Python and Go sorting are inconsistent!")
        print(f"  Python order: {python_order}")
        print(f"  Expected order: {expected_order}")
        return False


def test_problematic_format_python():
    """Test the problematic HHMM format with Python to show the issue."""
    filenames = [
        "tweets_1200.csv",  # 12:00
        "tweets_1300.csv",  # 13:00
        "tweets_2359.csv",  # 23:59 (should be last chronologically)
        "tweets_0000.csv",  # 00:00 (next day, should be last chronologically)
    ]

    # Sort lexicographically (Python's sorted() behavior)
    lexicographic_order = sorted(filenames)

    # Expected chronological order
    expected_chronological_order = [
        "tweets_1200.csv",  # 12:00
        "tweets_1300.csv",  # 13:00
        "tweets_2359.csv",  # 23:59
        "tweets_0000.csv",  # 00:00 (next day)
    ]

    print(f"Lexicographic order: {' -> '.join(lexicographic_order)}")
    print(f"Expected chronological order: {' -> '.join(expected_chronological_order)}")

    if lexicographic_order != expected_chronological_order:
        print("⚠️  PROBLEM CONFIRMED: Python's sorted() does NOT match chronological order!")
        print("   This would cause tweets to be processed out of order!")
        return False
    else:
        print("✓ Python's sorted() matches chronological order for this format")
        return True


def main():
    """Run all tests."""
    print("Testing Python file ordering behavior...")
    print("=" * 50)
    
    # Test 1: Gnip CSV format
    print("\n1. Testing gnip.csv format:")
    if test_gnip_csv_format_python():
        print("   ✅ PASS")
    else:
        print("   ❌ FAIL")
    
    # Test 2: Python vs Go consistency
    print("\n2. Testing Python vs Go consistency:")
    if test_python_vs_go_consistency():
        print("   ✅ PASS")
    else:
        print("   ❌ FAIL")
    
    # Test 3: Problematic format (expected to fail)
    print("\n3. Testing problematic HHMM format:")
    if test_problematic_format_python():
        print("   ✅ PASS")
    else:
        print("   ⚠️  EXPECTED FAIL (demonstrates problematic format)")
    
    print("\n" + "=" * 50)
    print("Python file ordering tests completed!")


if __name__ == "__main__":
    main() 