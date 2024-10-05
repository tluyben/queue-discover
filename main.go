package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
)

var (
	port         int
	metaDBPath   string
	workspacesDir string
)

func init() {
	flag.IntVar(&port, "port", 8080, "Port to run the server on")
	flag.StringVar(&metaDBPath, "metadb", "meta.db", "Path to the metadata SQLite database")
	flag.StringVar(&workspacesDir, "workspaces", "/home/workspaces", "Directory for workspace queue databases")
	flag.Parse()
}

func main() {
	// Open metadata database
	metaDB, err := sql.Open("sqlite3", metaDBPath)
	if err != nil {
		log.Fatalf("Error opening metadata database: %v", err)
	}
	defer metaDB.Close()

	// Initialize metadata tables
	if err := initMetaDB(metaDB); err != nil {
		log.Fatalf("Error initializing metadata database: %v", err)
	}

	// Create router and register routes
	r := mux.NewRouter()
	qh := &QueueHandler{db: metaDB, workspacesDir: workspacesDir}
	registerRoutes(r, qh)

	// Start the message processor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	qh.StartMessageProcessor(ctx)

	// Start the server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}

	go func() {
		log.Printf("Starting server on port %d", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down server...")
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
}

func initMetaDB(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS queues (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			cron TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS subscribers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			queue_id INTEGER NOT NULL,
			webhook_url TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (queue_id) REFERENCES queues(id) ON DELETE CASCADE
		);
	`)
	return err
}

func registerRoutes(r *mux.Router, qh *QueueHandler) {
	r.HandleFunc("/queues", qh.CreateQueue).Methods("POST")
	r.HandleFunc("/queues", qh.ListQueues).Methods("GET")
	r.HandleFunc("/queues/{id:[0-9]+}", qh.DeleteQueue).Methods("DELETE")
	r.HandleFunc("/queues/{id:[0-9]+}/empty", qh.EmptyQueue).Methods("POST")

	r.HandleFunc("/queues/{id:[0-9]+}/subscribers", qh.AddSubscriber).Methods("POST")
	r.HandleFunc("/queues/{id:[0-9]+}/subscribers", qh.ListSubscribers).Methods("GET")
	r.HandleFunc("/queues/{id:[0-9]+}/subscribers/{subId:[0-9]+}", qh.RemoveSubscriber).Methods("DELETE")

	r.HandleFunc("/queues/{id:[0-9]+}/messages", qh.SendMessage).Methods("POST")
}