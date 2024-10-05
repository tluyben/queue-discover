package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/maragudk/goqite"
)

type QueueHandler struct {
	db            *sql.DB
	workspacesDir string
}

func (h *QueueHandler) CreateQueue(w http.ResponseWriter, r *http.Request) {
	var q Queue
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := createQueue(h.db, h.workspacesDir, &q); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(q)
}

func (h *QueueHandler) ListQueues(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.ParseInt(r.URL.Query().Get("workspace_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid workspace_id", http.StatusBadRequest)
		return
	}

	queues, err := listQueues(h.db, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(queues)
}

func (h *QueueHandler) DeleteQueue(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid queue ID", http.StatusBadRequest)
		return
	}

	if err := deleteQueue(h.db, h.workspacesDir, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *QueueHandler) EmptyQueue(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid queue ID", http.StatusBadRequest)
		return
	}

	var workspaceID int64
	err = h.db.QueryRow("SELECT workspace_id FROM queues WHERE id = ?", id).Scan(&workspaceID)
	if err != nil {
		http.Error(w, "Queue not found", http.StatusNotFound)
		return
	}

	if err := emptyQueue(h.workspacesDir, id, workspaceID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *QueueHandler) AddSubscriber(w http.ResponseWriter, r *http.Request) {
	queueID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid queue ID", http.StatusBadRequest)
		return
	}

	var sub Subscriber
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := addSubscriber(h.db, queueID, sub.WebhookURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *QueueHandler) ListSubscribers(w http.ResponseWriter, r *http.Request) {
	queueID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid queue ID", http.StatusBadRequest)
		return
	}

	subscribers, err := listSubscribers(h.db, queueID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(subscribers)
}

func (h *QueueHandler) RemoveSubscriber(w http.ResponseWriter, r *http.Request) {
	subID, err := strconv.ParseInt(mux.Vars(r)["subId"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid subscriber ID", http.StatusBadRequest)
		return
	}

	if err := removeSubscriber(h.db, subID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
func (h *QueueHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid queue ID", http.StatusBadRequest)
		return
	}

	var message struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var workspaceID int64
	err = h.db.QueryRow("SELECT workspace_id FROM queues WHERE id = ?", id).Scan(&workspaceID)
	if err != nil {
		http.Error(w, "Queue not found", http.StatusNotFound)
		return
	}

	if err := sendMessage(h.workspacesDir, id, workspaceID, []byte(message.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// StartMessageProcessor starts a background worker to process messages and notify subscribers
func (h *QueueHandler) StartMessageProcessor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.processMessages()
			}
		}
	}()
}

func (h *QueueHandler) processMessages() {
	rows, err := h.db.Query("SELECT id, workspace_id FROM queues")
	if err != nil {
		log.Printf("Error querying queues: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var queueID, workspaceID int64
		if err := rows.Scan(&queueID, &workspaceID); err != nil {
			log.Printf("Error scanning queue row: %v", err)
			continue
		}

		if err := h.processQueueMessages(queueID, workspaceID); err != nil {
			log.Printf("Error processing messages for queue %d: %v", queueID, err)
		}
	}
}

func (h *QueueHandler) processQueueMessages(queueID, workspaceID int64) error {
	queuePath := filepath.Join(h.workspacesDir, fmt.Sprintf("%d", workspaceID), "queues", fmt.Sprintf("%d.sqlite", queueID))
	queueDB, err := sql.Open("sqlite3", queuePath)
	if err != nil {
		return err
	}
	defer queueDB.Close()

	q := goqite.New(goqite.NewOpts{
		DB:   queueDB,
		Name: fmt.Sprintf("queue_%d", queueID),
	})

	for {
		msg, err := q.Receive(context.Background())
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil // No more messages in the queue
			}
			return err
		}

		if err := h.notifySubscribers(queueID, msg.Body); err != nil {
			log.Printf("Error notifying subscribers for queue %d: %v", queueID, err)
		}

		if err := q.Delete(context.Background(), msg.ID); err != nil {
			log.Printf("Error deleting message %s from queue %d: %v", msg.ID, queueID, err)
		}
	}
}

func (h *QueueHandler) notifySubscribers(queueID int64, message []byte) error {
	subscribers, err := listSubscribers(h.db, queueID)
	if err != nil {
		return err
	}

	for _, sub := range subscribers {
		go func(webhookURL string) {
			resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(message))
			if err != nil {
				log.Printf("Error sending message to webhook %s: %v", webhookURL, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				log.Printf("Webhook %s returned non-2xx status code: %d", webhookURL, resp.StatusCode)
			}
		}(sub.WebhookURL)
	}

	return nil
} 