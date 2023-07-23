package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
)

const (
	port        = 8080
	authToken   = "happy-blue-falcon-cheese"
	dbName      = "saigon-data.db"
	dataTimeout = 600 * time.Second
)

type Message struct {
	ID             int
	Hostname       string
	OS             string
	Kernel         string
	Uptime         string
	Shell          string
	CPU            string
	CPUPercentage  string
	MemStats       string
	RAMPercentage  string
	TotalDiskSpace string
	FreeDiskSpace  string
	UsedDiskSpace  string
	SystemArch     string
	AuthToken      string
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	// Open SQLite database
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Initialize the database
	initializeDB(db)

	router := mux.NewRouter()

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleConnection(w, r, db)
	})

	router.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		handleLatestData(w, r, db)
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), router))

	log.Printf("Server is listening on port %d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func initializeDB(db *sql.DB) {
	query := `
	CREATE TABLE IF NOT EXISTS system_data (
		id INTEGER PRIMARY KEY,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		hostname TEXT,
		os TEXT,
		kernel TEXT,
		uptime TEXT,
		shell TEXT,
		cpu TEXT,
		cpu_percentage TEXT,
		mem_stats TEXT,
		ram_percentage TEXT,
		total_disk_space TEXT,
		free_disk_space TEXT,
		used_disk_space TEXT,
		system_arch TEXT
	);
	`
	_, err := db.Exec(query)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
}

func handleConnection(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	// Upgrade the HTTP server connection to the WebSocket protocol
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	for {
		var msg Message

		// Set a deadline for reading the next pong message from the client
		err := conn.SetReadDeadline(time.Now().Add(dataTimeout))
		if err != nil {
			log.Printf("Failed to set read deadline: %v", err)
			return
		}

		// Read message from client
		err = conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseAbnormalClosure) {
				log.Printf("Client disconnected unexpectedly")
			} else {
				log.Printf("Failed to read message: %v", err)
			}
			return
		}

		// Validate auth token
		if msg.AuthToken != authToken {
			log.Printf("Invalid auth token: %v", msg.AuthToken)
			return
		}

		// Insert data into database
		_, err = db.Exec(`INSERT INTO system_data (hostname, os, kernel, uptime, shell, cpu, cpu_percentage, mem_stats, ram_percentage, total_disk_space, free_disk_space, used_disk_space, system_arch) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			msg.Hostname, msg.OS, msg.Kernel, msg.Uptime, msg.Shell, msg.CPU, msg.CPUPercentage, msg.MemStats, msg.RAMPercentage, msg.TotalDiskSpace, msg.FreeDiskSpace, msg.UsedDiskSpace, msg.SystemArch)
		if err != nil {
			log.Printf("Failed to insert data into database: %v", err)
			return
		}
		fmt.Printf("OK: %s %s\n", time.Now().String()[:36], msg.Hostname)
	}
}
func handleLatestData(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	rows, err := db.Query(`
	SELECT *
	FROM system_data
	WHERE (hostname, timestamp) IN (
		SELECT hostname, MAX(timestamp)
		FROM system_data
		GROUP BY hostname
	)
	ORDER BY hostname
	`)
	if err != nil {
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		log.Printf("Failed to query database: %v", err)
		return
	}
	defer rows.Close()

	// Create a slice to hold our data
	data := []Message{}

	// Iterate over rows
	for rows.Next() {
		var msg Message
		err := rows.Scan(&msg.ID, &msg.Hostname, &msg.OS, &msg.Kernel, &msg.Uptime, &msg.Shell, &msg.CPU, &msg.CPUPercentage, &msg.MemStats, &msg.RAMPercentage, &msg.TotalDiskSpace, &msg.FreeDiskSpace, &msg.UsedDiskSpace, &msg.SystemArch, &msg.AuthToken)
		if err != nil {
			http.Error(w, "Failed to scan row", http.StatusInternalServerError)
			log.Printf("Failed to scan row: %v", err)
			return
		}
		msg.AuthToken = ""
		msg.ID = 0
		data = append(data, msg)
	}

	// At this point, 'data' contains the latest entry for each host
	// You can pass this data to the latest.html template

	// Parse the latest.html template
	tmpl, err := template.ParseFiles("latest.html")
	if err != nil {
		http.Error(w, "Failed to parse template", http.StatusInternalServerError)
		log.Printf("Failed to parse template: %v", err)
		return
	}

	// Execute the template with the data
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Failed to execute template", http.StatusInternalServerError)
		log.Printf("Failed to execute template: %v", err)
		return
	}
}
