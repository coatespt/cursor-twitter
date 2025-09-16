#!/bin/bash

# Twitter Data Processing Pipeline Setup Script
# This script automates the installation process for a complete stranger

set -e  # Exit on any error

echo "🚀 Twitter Data Processing Pipeline Setup"
echo "=========================================="

# Check if running as root
if [[ $EUID -eq 0 ]]; then
   echo "❌ This script should not be run as root"
   exit 1
fi

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
echo "📋 Checking prerequisites..."

if ! command_exists go; then
    echo "❌ Go is not installed. Please install Go 1.19+ first."
    echo "   Visit: https://golang.org/doc/install"
    exit 1
fi

if ! command_exists python3; then
    echo "❌ Python 3 is not installed. Please install Python 3.8+ first."
    exit 1
fi

if ! command_exists psql; then
    echo "❌ PostgreSQL client is not installed. Please install PostgreSQL first."
    exit 1
fi

if ! command_exists git; then
    echo "❌ Git is not installed. Please install Git first."
    exit 1
fi

echo "✅ All prerequisites found"

# Install Go dependencies
echo "📦 Installing Go dependencies..."
go mod tidy

# Install Python dependencies
echo "📦 Installing Python dependencies..."
cd parser
pip3 install -r requirements.txt
cd ..

# Check if PostgreSQL is running
echo "🗄️  Checking PostgreSQL connection..."
if ! pg_isready -h localhost -p 5432 >/dev/null 2>&1; then
    echo "❌ PostgreSQL is not running. Please start PostgreSQL first."
    echo "   On Ubuntu/Debian: sudo systemctl start postgresql"
    echo "   On macOS: brew services start postgresql"
    exit 1
fi

# Create database and user (if they don't exist)
echo "🗄️  Setting up database..."
PGPASSWORD=aardvark1 psql -h localhost -U petercoates -d x_twitter -c "\q" 2>/dev/null || {
    echo "Creating database and user..."
    sudo -u postgres psql << EOF
CREATE DATABASE x_twitter;
CREATE USER petercoates WITH PASSWORD 'aardvark1';
GRANT ALL PRIVILEGES ON DATABASE x_twitter TO petercoates;
\q
EOF
}

# Create database schema
echo "🗄️  Creating database schema..."
PGPASSWORD=aardvark1 psql -h localhost -U petercoates -d x_twitter -f src/sql_loader/create_new_tables.sql

# Build the system
echo "🔨 Building the system..."
make build-all

# Run tests
echo "🧪 Running tests..."
make test

echo ""
echo "🎉 Setup complete!"
echo ""
echo "To run the system:"
echo "  1. Main pipeline: ./twitter-pipeline -config config/config.yaml"
echo "  2. Display: cd display && ./cursor-twitter-display"
echo "  3. AI Display: cd ai_display && ./ai_display ../config/ai_display.yaml"
echo ""
echo "For more information, see INSTALLATION.md"
