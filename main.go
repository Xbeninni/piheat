package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type TemperatureReading struct {
	Temperature float64 `json:"temperature"`
	Timestamp   string  `json:"timestamp"`
}

func getTemperature() (float64, error) {
	data, err := ioutil.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, err
	}

	tempStr := strings.TrimSpace(string(data))
	tempMilliCelsius, err := strconv.Atoi(tempStr)
	if err != nil {
		return 0, err
	}

	tempCelsius := float64(tempMilliCelsius) / 1000.0
	return tempCelsius, nil
}

func temperatureHandler(w http.ResponseWriter, r *http.Request) {
	temp, err := getTemperature()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading temperature: %v", err), http.StatusInternalServerError)
		return
	}

	reading := TemperatureReading{
		Temperature: temp,
		Timestamp:   time.Now().Format("2006-01-02 15:04:05"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reading)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Pi Temperature Monitor</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 600px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; text-align: center; }
        .temp-display { font-size: 48px; text-align: center; margin: 30px 0; color: #007bff; }
        .timestamp { text-align: center; color: #666; margin-bottom: 20px; }
        .status { text-align: center; padding: 10px; border-radius: 4px; margin: 20px 0; }
        .normal { background: #d4edda; color: #155724; }
        .warning { background: #fff3cd; color: #856404; }
        .danger { background: #f8d7da; color: #721c24; }
        button { background: #007bff; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; display: block; margin: 20px auto; }
        button:hover { background: #0056b3; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🌡️ Raspberry Pi Temperature Monitor</h1>
        <div id="temperature" class="temp-display">Loading...</div>
        <div id="timestamp" class="timestamp"></div>
        <div id="status" class="status"></div>
        <button onclick="updateTemperature()">Refresh</button>
    </div>

    <script>
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
                        statusDiv.textContent = 'Temperature Normal';
                    } else if (temp < 75) {
                        statusDiv.className = 'status warning';
                        statusDiv.textContent = 'Temperature Warning';
                    } else {
                        statusDiv.className = 'status danger';
                        statusDiv.textContent = 'Temperature Critical!';
                    }
                })
                .catch(error => {
                    console.error('Error:', error);
                    document.getElementById('temperature').textContent = 'Error';
                    document.getElementById('timestamp').textContent = 'Failed to fetch data';
                });
        }

        updateTemperature();
        setInterval(updateTemperature, 5000);
    </script>
</body>
</html>`

	t := template.Must(template.New("index").Parse(tmpl))
	t.Execute(w, nil)
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/api/temperature", temperatureHandler)

	fmt.Println("Pi Temperature Monitor starting on :8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}