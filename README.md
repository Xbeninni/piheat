# Pi Temperature Monitor

A lightweight web application for monitoring Raspberry Pi CPU temperature in real-time through a clean web interface.

## Features

- **Real-time temperature monitoring** - Displays current CPU temperature
- **Web-based dashboard** - Clean, responsive interface accessible via browser
- **JSON API** - REST endpoint for temperature data
- **Status indicators** - Visual alerts for temperature ranges:
  - Normal: < 60°C (green)
  - Warning: 60-75°C (yellow) 
  - Critical: > 75°C (red)
- **Auto-refresh** - Updates every 5 seconds automatically

## Requirements

- Raspberry Pi running Linux
- Go 1.18 or later
- Access to `/sys/class/thermal/thermal_zone0/temp` (standard on most Pi distributions)

## Installation

1. Clone or download this repository
2. Build the application:
   ```bash
   go build -o piheat main.go
   ```

## Usage

1. Run the application:
   ```bash
   ./piheat
   ```

2. Open your browser and navigate to:
   ```
   http://localhost:8082
   ```

3. The dashboard will display:
   - Current CPU temperature
   - Last update timestamp
   - Temperature status with color coding
   - Manual refresh button

## API Endpoints

### GET /
- Returns the web dashboard interface

### GET /api/temperature
- Returns JSON with current temperature data
- Response format:
  ```json
  {
    "temperature": 45.2,
    "timestamp": "2024-01-15 14:30:25"
  }
  ```

## Configuration

The application runs on port `8082` by default. To change the port, modify the `main()` function in `main.go`.

## Temperature Thresholds

- **Normal**: Below 60°C - Optimal operating range
- **Warning**: 60-75°C - Consider improving cooling
- **Critical**: Above 75°C - Risk of thermal throttling

## Troubleshooting

- **Permission denied**: Ensure the application has read access to `/sys/class/thermal/thermal_zone0/temp`
- **File not found**: Verify you're running on a Raspberry Pi or compatible system
- **Port in use**: Change the port number in `main.go` if 8082 is already occupied

## License

This project is open source and available under standard terms.