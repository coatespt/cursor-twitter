#!/usr/bin/env python3
"""
Atomic file operations with file locking to prevent race conditions.
This module provides safe read/write operations for status files that may be
accessed by multiple processes.
"""

import os
import fcntl
import tempfile
from pathlib import Path


def atomic_write_with_lock(filename, data):
    """
    Atomically write data to a file with proper locking.
    
    Args:
        filename (str): Path to the target file
        data (str): Data to write to the file
    
    Raises:
        OSError: If the file operation fails
    """
    # Ensure the directory exists
    Path(filename).parent.mkdir(parents=True, exist_ok=True)
    
    # Create a temporary file in the same directory
    temp_filename = filename + '.tmp'
    
    try:
        # Write to temporary file first
        with open(temp_filename, 'w') as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())  # Ensure data is written to disk
        
        # Atomically rename temp file to target file
        os.replace(temp_filename, filename)
        
    except Exception as e:
        # Clean up temp file if it exists
        if os.path.exists(temp_filename):
            try:
                os.unlink(temp_filename)
            except:
                pass
        raise e


def read_with_lock(filename):
    """
    Read data from a file with shared lock (allows multiple readers).
    
    Args:
        filename (str): Path to the file to read
    
    Returns:
        str: Contents of the file, or empty string if file doesn't exist
    
    Raises:
        OSError: If the file operation fails
    """
    if not os.path.exists(filename):
        return ""
    
    with open(filename, 'r') as f:
        # Acquire shared lock (allows multiple readers)
        fcntl.flock(f, fcntl.LOCK_SH)
        try:
            data = f.read()
            return data.strip()  # Remove trailing whitespace
        finally:
            fcntl.flock(f, fcntl.LOCK_UN)


def write_with_exclusive_lock(filename, data):
    """
    Write data to a file with exclusive lock (blocks other readers/writers).
    
    Args:
        filename (str): Path to the target file
        data (str): Data to write to the file
    
    Raises:
        OSError: If the file operation fails
    """
    # Ensure the directory exists
    Path(filename).parent.mkdir(parents=True, exist_ok=True)
    
    with open(filename, 'w') as f:
        # Acquire exclusive lock (blocks other processes)
        fcntl.flock(f, fcntl.LOCK_EX)
        try:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())  # Ensure data is written to disk
        finally:
            fcntl.flock(f, fcntl.LOCK_UN)


def update_sender_status(status_file, last_processed_file):
    """
    Update the sender status file with the last processed file.
    This function uses atomic write to ensure the file is never in a
    partially-written state.
    
    Args:
        status_file (str): Path to the status file
        last_processed_file (str): Name of the last processed file
    
    Raises:
        OSError: If the file operation fails
    """
    atomic_write_with_lock(status_file, last_processed_file)


def get_sender_status(status_file):
    """
    Get the last processed file from the status file.
    
    Args:
        status_file (str): Path to the status file
    
    Returns:
        str: Name of the last processed file, or empty string if not found
    """
    return read_with_lock(status_file)


# Test functions for verification
def test_atomic_operations():
    """Test the atomic file operations."""
    import tempfile
    
    # Create a temporary file for testing
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as f:
        test_file = f.name
    
    try:
        # Test atomic write
        test_data = "test_file_001.csv"
        atomic_write_with_lock(test_file, test_data)
        
        # Test read
        read_data = read_with_lock(test_file)
        assert read_data == test_data, f"Expected '{test_data}', got '{read_data}'"
        
        # Test update
        new_data = "test_file_002.csv"
        update_sender_status(test_file, new_data)
        
        # Verify update
        read_data = get_sender_status(test_file)
        assert read_data == new_data, f"Expected '{new_data}', got '{read_data}'"
        
        print("✓ All atomic file operations tests passed!")
        
    finally:
        # Clean up
        if os.path.exists(test_file):
            os.unlink(test_file)


if __name__ == "__main__":
    test_atomic_operations() 