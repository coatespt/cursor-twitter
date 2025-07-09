#!/usr/bin/env python3
"""
Test script to verify status tracking functionality.
"""

import os
import tempfile
import yaml
from atomic_file import update_sender_status, get_sender_status


def test_status_tracking():
    """Test the status tracking functionality."""
    
    # Create a temporary config file
    config_data = {
        'sender': {
            'status_file': '/tmp/test_sender_status.txt'
        }
    }
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        yaml.dump(config_data, f)
        config_file = f.name
    
    try:
        # Test status file operations
        status_file = config_data['sender']['status_file']
        
        # Initially should be empty
        status = get_sender_status(status_file)
        print(f"Initial status: '{status}'")
        
        # Update with a file name
        test_file = "/path/to/test_file_001.csv"
        update_sender_status(status_file, test_file)
        
        # Read back the status
        status = get_sender_status(status_file)
        print(f"After update status: '{status}'")
        assert status == test_file, f"Expected '{test_file}', got '{status}'"
        
        # Update with another file name
        test_file2 = "/path/to/test_file_002.csv"
        update_sender_status(status_file, test_file2)
        
        # Read back the status
        status = get_sender_status(status_file)
        print(f"After second update status: '{status}'")
        assert status == test_file2, f"Expected '{test_file2}', got '{status}'"
        
        print("✓ Status tracking test passed!")
        
    finally:
        # Clean up
        if os.path.exists(config_file):
            os.unlink(config_file)
        if os.path.exists(status_file):
            os.unlink(status_file)


if __name__ == "__main__":
    test_status_tracking() 