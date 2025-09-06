#!/bin/bash
# Run AI Analysis Progress Report for sept_4_ptc

echo "Running AI Analysis Progress Report for sept_4_ptc..."
echo "=================================================="

psql -h 192.168.1.76 -U petercoates -d x_twitter -f ai_analysis_progress.sql

echo ""
echo "Report complete!"
