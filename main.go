package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type TemperatureReading struct {
	Temperature float64 `json:"temperature"`
	Timestamp   string  `json:"timestamp"`
}

type ChartDataPoint struct {
	Temperature float64 `json:"temperature"`
	Timestamp   string  `json:"timestamp"`
	UnixTime    int64   `json:"unixTime"`
}

var db *sql.DB

func initDatabase() {
	var err error
	db, err = sql.Open("sqlite3", "./temperature.db")
	if err != nil {
		log.Fatal(err)
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS temperature_readings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		temperature REAL NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal(err)
	}

	// Create index for faster queries
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_timestamp ON temperature_readings(timestamp);")
	if err != nil {
		log.Fatal(err)
	}
}

func saveTemperature(temp float64) error {
	_, err := db.Exec("INSERT INTO temperature_readings (temperature) VALUES (?)", temp)
	return err
}

func getTemperature() (float64, error) {
	// Try to read from Raspberry Pi thermal zone first
	data, err := ioutil.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err == nil {
		tempStr := strings.TrimSpace(string(data))
		tempMilliCelsius, err := strconv.Atoi(tempStr)
		if err == nil {
			tempCelsius := float64(tempMilliCelsius) / 1000.0
			return tempCelsius, nil
		}
	}

	// If not available (not on Pi), generate dummy temperature data
	// Simulate realistic CPU temperature with some variation
	baseTemp := 55.0
	variation := 10.0 * (0.5 - float64(time.Now().Unix()%60)/60.0) // Varies over minute
	noise := float64((time.Now().UnixNano()/1000000)%10-5) * 0.2   // Small random noise
	temp := baseTemp + variation + noise
	
	// Ensure temperature stays in reasonable range
	if temp < 40 {
		temp = 40
	}
	if temp > 80 {
		temp = 80
	}
	
	return temp, nil
}

func temperatureHandler(w http.ResponseWriter, r *http.Request) {
	temp, err := getTemperature()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading temperature: %v", err), http.StatusInternalServerError)
		return
	}

	// Save to database
	if err := saveTemperature(temp); err != nil {
		log.Printf("Error saving temperature to database: %v", err)
	}

	reading := TemperatureReading{
		Temperature: temp,
		Timestamp:   time.Now().Format("2006-01-02 15:04:05"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reading)
}

func chartDataHandler(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "day"
	}

	var query string
	var timeFormat string

	switch period {
	case "day":
		query = "SELECT temperature, timestamp FROM temperature_readings WHERE timestamp >= datetime('now', '-1 day') ORDER BY timestamp"
		timeFormat = "15:04"
	case "week":
		query = "SELECT AVG(temperature) as temperature, datetime(timestamp, 'start of hour') as timestamp FROM temperature_readings WHERE timestamp >= datetime('now', '-7 days') GROUP BY datetime(timestamp, 'start of hour') ORDER BY timestamp"
		timeFormat = "01-02 15:04"
	case "month":
		query = "SELECT AVG(temperature) as temperature, date(timestamp) as timestamp FROM temperature_readings WHERE timestamp >= datetime('now', '-1 month') GROUP BY date(timestamp) ORDER BY timestamp"
		timeFormat = "01-02"
	case "year":
		query = "SELECT AVG(temperature) as temperature, date(timestamp, 'start of month') as timestamp FROM temperature_readings WHERE timestamp >= datetime('now', '-1 year') GROUP BY date(timestamp, 'start of month') ORDER BY timestamp"
		timeFormat = "2006-01"
	default:
		query = "SELECT temperature, timestamp FROM temperature_readings WHERE timestamp >= datetime('now', '-1 day') ORDER BY timestamp"
		timeFormat = "15:04"
	}

	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error querying database: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var data []ChartDataPoint
	for rows.Next() {
		var temp float64
		var timestampStr string
		if err := rows.Scan(&temp, &timestampStr); err != nil {
			continue
		}

		// Parse timestamp - try multiple formats
		var parsedTime time.Time
		var parseErr error
		
		// Try RFC3339 format first (ISO format from SQLite)
		parsedTime, parseErr = time.Parse(time.RFC3339, timestampStr)
		if parseErr != nil {
			// Try standard datetime format
			parsedTime, parseErr = time.Parse("2006-01-02 15:04:05", timestampStr)
			if parseErr != nil {
				// Try date only format
				parsedTime, parseErr = time.Parse("2006-01-02", timestampStr)
				if parseErr != nil {
					continue
				}
			}
		}

		data = append(data, ChartDataPoint{
			Temperature: temp,
			Timestamp:   parsedTime.Format(timeFormat),
			UnixTime:    parsedTime.Unix(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Pi CPU Temperature Monitor</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { 
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
            background: #f5f5f5;
            min-height: 100vh;
            padding: 20px;
        }
        .container { 
            max-width: 1200px; 
            margin: 0 auto; 
            background: white; 
            border-radius: 20px; 
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(45deg, #2196F3, #21CBF3);
            color: white;
            padding: 30px;
            text-align: center;
        }
        h1 { 
            font-size: 2.5em; 
            margin-bottom: 10px;
            text-shadow: 0 2px 4px rgba(0,0,0,0.3);
        }
        .subtitle {
            font-size: 1.1em;
            opacity: 0.9;
        }
        .dashboard {
            display: grid;
            grid-template-columns: 1fr 2fr;
            gap: 30px;
            padding: 30px;
        }
        .current-temp {
            background: white;
            border-radius: 15px;
            padding: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.1);
            text-align: center;
        }
        .temp-display { 
            font-size: 4em; 
            font-weight: bold;
            margin: 20px 0;
            background: linear-gradient(45deg, #2196F3, #21CBF3);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        .timestamp { 
            color: #666; 
            margin-bottom: 20px;
            font-size: 0.9em;
        }
        .status { 
            padding: 15px; 
            border-radius: 10px; 
            margin: 20px 0;
            font-weight: bold;
            transition: all 0.3s ease;
        }
        .normal { background: linear-gradient(45deg, #4CAF50, #45a049); color: white; }
        .warning { background: linear-gradient(45deg, #FF9800, #F57C00); color: white; }
        .danger { background: linear-gradient(45deg, #f44336, #d32f2f); color: white; }
        .chart-container {
            background: white;
            border-radius: 15px;
            padding: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.1);
        }
        .time-buttons {
            display: flex;
            gap: 10px;
            margin-bottom: 20px;
            flex-wrap: wrap;
        }
        .time-btn {
            background: linear-gradient(45deg, #e3f2fd, #bbdefb);
            border: 2px solid #2196F3;
            color: #1976D2;
            padding: 12px 24px;
            border-radius: 25px;
            cursor: pointer;
            font-weight: bold;
            transition: all 0.3s ease;
            font-size: 0.9em;
        }
        .time-btn:hover {
            background: linear-gradient(45deg, #2196F3, #21CBF3);
            color: white;
            transform: translateY(-2px);
            box-shadow: 0 5px 15px rgba(33, 150, 243, 0.4);
        }
        .time-btn.active {
            background: linear-gradient(45deg, #2196F3, #21CBF3);
            color: white;
            box-shadow: 0 5px 15px rgba(33, 150, 243, 0.4);
        }
        .refresh-btn {
            background: linear-gradient(45deg, #4CAF50, #45a049);
            color: white;
            border: none;
            padding: 15px 30px;
            border-radius: 25px;
            cursor: pointer;
            font-weight: bold;
            margin-top: 20px;
            transition: all 0.3s ease;
        }
        .refresh-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 5px 15px rgba(76, 175, 80, 0.4);
        }
        #temperatureChart {
            height: 400px !important;
        }
        .loading {
            text-align: center;
            color: #666;
            font-style: italic;
        }
        @media (max-width: 768px) {
            .dashboard {
                grid-template-columns: 1fr;
                gap: 20px;
                padding: 20px;
            }
            .temp-display { font-size: 3em; }
            h1 { font-size: 2em; }
            .time-buttons { justify-content: center; }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🖥️ Raspberry Pi CPU Temperature Monitor</h1>
            <div class="subtitle">Real-time CPU temperature monitoring with historical data analysis</div>
        </div>
        
        <div class="dashboard">
            <div class="current-temp">
                <h2>Current CPU Temperature</h2>
                <div id="temperature" class="temp-display">Loading...</div>
                <div id="timestamp" class="timestamp"></div>
                <div id="status" class="status"></div>
                <button class="refresh-btn" onclick="updateTemperature()">🔄 Refresh</button>
            </div>
            
            <div class="chart-container">
                <h2>CPU Temperature History</h2>
                <div class="time-buttons">
                    <button class="time-btn active" onclick="changePeriod('day', this)">📅 Today</button>
                    <button class="time-btn" onclick="changePeriod('week', this)">📊 Week</button>
                    <button class="time-btn" onclick="changePeriod('month', this)">📈 Month</button>
                    <button class="time-btn" onclick="changePeriod('year', this)">📉 Year</button>
                </div>
                <canvas id="temperatureChart"></canvas>
            </div>
        </div>
    </div>

    <script>
        let chart;
        let currentPeriod = 'day';

        function initChart() {
            const ctx = document.getElementById('temperatureChart').getContext('2d');
            chart = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: [],
                    datasets: [{
                        label: 'CPU Temperature (°C)',
                        data: [],
                        borderColor: 'rgb(33, 150, 243)',
                        backgroundColor: 'rgba(33, 150, 243, 0.1)',
                        borderWidth: 3,
                        fill: true,
                        tension: 0.4,
                        pointBackgroundColor: 'rgb(33, 150, 243)',
                        pointBorderColor: 'white',
                        pointBorderWidth: 2,
                        pointRadius: 4,
                        pointHoverRadius: 6
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        legend: {
                            display: true,
                            position: 'top'
                        }
                    },
                    scales: {
                        x: {
                            display: true,
                            title: {
                                display: true,
                                text: 'Time'
                            },
                            grid: {
                                color: 'rgba(0,0,0,0.1)'
                            }
                        },
                        y: {
                            display: true,
                            title: {
                                display: true,
                                text: 'CPU Temperature (°C)'
                            },
                            grid: {
                                color: 'rgba(0,0,0,0.1)'
                            },
                            beginAtZero: false
                        }
                    },
                    interaction: {
                        intersect: false,
                        mode: 'index'
                    }
                }
            });
        }

        function updateChart(period = currentPeriod) {
            fetch('/api/chart-data?period=' + period)
                .then(response => response.json())
                .then(data => {
                    chart.data.labels = data.map(d => d.timestamp);
                    chart.data.datasets[0].data = data.map(d => d.temperature);
                    chart.update();
                })
                .catch(error => {
                    console.error('Error updating chart:', error);
                });
        }

        function updateTemperature() {
            fetch('/api/temperature')
                .then(response => response.json())
                .then(data => {
                    document.getElementById('temperature').textContent = data.temperature.toFixed(1) + '°C';
                    document.getElementById('timestamp').textContent = 'Last updated: ' + data.timestamp;
                    
                    const statusDiv = document.getElementById('status');
                    const temp = data.temperature;
                    
                    if (temp < 60) {
                        statusDiv.className = 'status normal';
                        statusDiv.textContent = '✅ Temperature Normal';
                    } else if (temp < 75) {
                        statusDiv.className = 'status warning';
                        statusDiv.textContent = '⚠️ Temperature Warning';
                    } else {
                        statusDiv.className = 'status danger';
                        statusDiv.textContent = '🔥 Temperature Critical!';
                    }
                    
                    // Update chart if we're on current day view
                    if (currentPeriod === 'day') {
                        updateChart();
                    }
                })
                .catch(error => {
                    console.error('Error:', error);
                    document.getElementById('temperature').textContent = 'Error';
                    document.getElementById('timestamp').textContent = 'Failed to fetch data';
                });
        }

        function changePeriod(period, button) {
            currentPeriod = period;
            
            // Update button states
            document.querySelectorAll('.time-btn').forEach(btn => btn.classList.remove('active'));
            button.classList.add('active');
            
            // Update chart
            updateChart(period);
        }

        // Initialize everything
        initChart();
        updateTemperature();
        updateChart();
        
        // Auto-refresh current temperature every 5 seconds
        setInterval(updateTemperature, 5000);
        
        // Auto-refresh chart every 30 seconds for day view
        setInterval(() => {
            if (currentPeriod === 'day') {
                updateChart();
            }
        }, 30000);
    </script>
</body>
</html>`

	t := template.Must(template.New("index").Parse(tmpl))
	t.Execute(w, nil)
}


func main() {
	initDatabase()
	defer db.Close()

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/api/temperature", temperatureHandler)
	http.HandleFunc("/api/chart-data", chartDataHandler)

	log.Println("Pi Temperature Monitor starting on :8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}