# Installation Guide

This guide will help a complete stranger install and run the Twitter Data Processing Pipeline from scratch.

## Prerequisites

### System Requirements
- **Operating System**: Linux, macOS, or Windows with WSL
- **Go**: Version 1.19 or later
- **Python**: Version 3.8 or later (for parser components)
- **PostgreSQL**: Version 12 or later
- **Git**: For cloning the repository

### Install Prerequisites

#### Ubuntu/Debian:
```bash
sudo apt update
sudo apt install golang-go python3 python3-pip postgresql postgresql-contrib git
```

#### macOS (with Homebrew):
```bash
brew install go python3 postgresql git
```

#### Windows (with Chocolatey):
```powershell
choco install golang python postgresql git
```

## Installation Steps

### 1. Clone the Repository
```bash
git clone <repository-url>
cd cursor-twitter
```

### 2. Install Go Dependencies
```bash
go mod tidy
```

### 3. Install Python Dependencies
```bash
cd parser
pip3 install -r requirements.txt
cd ..
```

### 4. Set Up PostgreSQL Database

#### Create Database and User
```bash
# Connect to PostgreSQL as superuser
sudo -u postgres psql

# Create database and user
CREATE DATABASE x_twitter;
CREATE USER petercoates WITH PASSWORD 'aardvark1';
GRANT ALL PRIVILEGES ON DATABASE x_twitter TO petercoates;
\q
```

#### Create Database Schema
```bash
# Create the database schema
PGPASSWORD=aardvark1 psql -h localhost -U petercoates -d x_twitter -f src/sql_loader/create_new_tables.sql
```

### 5. Configure the System

#### Update Database Configuration
Edit `config/database.yaml`:
```yaml
host: "localhost"  # or your PostgreSQL server IP
port: 5432
user: "petercoates"
password: "aardvark1"
name: "x_twitter"
```

#### Update AI Display Configuration
Edit `config/ai_display.yaml`:
```yaml
database:
  host: "localhost"  # or your PostgreSQL server IP
  port: 5432
  user: "petercoates"
  password: "aardvark1"
  name: "x_twitter"
```

### 6. Build the System
```bash
# Build all components
make build-all

# Or build individually:
make build              # Main pipeline
make build-display      # Display component
make build-ai-display   # AI display component
```

### 7. Test the Installation
```bash
# Run tests
make test

# Test database connection
PGPASSWORD=aardvark1 psql -h localhost -U petercoates -d x_twitter -c "\dt"
```

## Running the System

### 1. Start the Main Pipeline
```bash
./twitter-pipeline -config config/config.yaml
```

### 2. Start the Display Component
```bash
cd display
./cursor-twitter-display
# Open browser to http://localhost:8080
```

### 3. Start the AI Display Component
```bash
cd ai_display
./ai_display ../config/ai_display.yaml
# Open browser to http://localhost:8081
```

## Configuration Files

### Main Configuration (`config/config.yaml`)
- Database connection settings
- Processing parameters
- Logging configuration

### Display Configuration (`display/config.yaml`)
- Display server settings
- Data file paths

### AI Display Configuration (`config/ai_display.yaml`)
- AI analysis settings
- Database connection for AI features

## Troubleshooting

### Database Connection Issues
1. Verify PostgreSQL is running: `sudo systemctl status postgresql`
2. Check database exists: `PGPASSWORD=aardvark1 psql -h localhost -U petercoates -d x_twitter -c "\l"`
3. Verify user permissions: `PGPASSWORD=aardvark1 psql -h localhost -U petercoates -d x_twitter -c "\dt"`

### Build Issues
1. Check Go version: `go version` (should be 1.19+)
2. Clean and rebuild: `make clean && make build-all`
3. Check dependencies: `go mod tidy`

### Python Issues
1. Check Python version: `python3 --version` (should be 3.8+)
2. Reinstall dependencies: `cd parser && pip3 install -r requirements.txt`

## Data Requirements

The system expects Twitter data in JSON format. Sample data files are provided in `testdata/` for testing.

## Support

For issues or questions:
1. Check the logs in the `logs/` directory
2. Review the configuration files
3. Run tests to verify installation: `make test`

## Security Notes

- Change default passwords in production
- Use environment variables for sensitive configuration
- Ensure PostgreSQL is properly secured
- Consider using SSL for database connections in production
