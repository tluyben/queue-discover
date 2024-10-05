package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maragudk/goqite"
)

type Queue struct {
	ID          int64
	WorkspaceID int64
	Name        string
	Description string
	Cron        string
}

type Subscriber struct {
	ID         int64
	QueueID    int64
	WebhookURL string
}

func createQueue(db *sql.DB, workspacesDir string, q *Queue) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec("INSERT INTO queues (workspace_id, name, description, cron) VALUES (?, ?, ?, ?)",
		q.WorkspaceID, q.Name, q.Description, q.Cron)
	if err != nil {
		return err
	}

	q.ID, err = result.LastInsertId()
	if err != nil {
		return err
	}

	// Create SQLite file for the queue
	queueDir := filepath.Join(workspacesDir, fmt.Sprintf("%d", q.WorkspaceID), "queues")
	if err := os.MkdirAll(queueDir, 0755); err != nil {
		return err
	}

	queueDB, err := sql.Open("sqlite3", filepath.Join(queueDir, fmt.Sprintf("%d.sqlite", q.ID)))
	if err != nil {
		return err
	}
	defer queueDB.Close()

	if err := goqite.Setup(context.Background(), queueDB); err != nil {
		return err
	}

	return tx.Commit()
}

func listQueues(db *sql.DB, workspaceID int64) ([]Queue, error) {
	rows, err := db.Query("SELECT id, workspace_id, name, description, cron FROM queues WHERE workspace_id = ?", workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queues []Queue
	for rows.Next() {
		var q Queue
		if err := rows.Scan(&q.ID, &q.WorkspaceID, &q.Name, &q.Description, &q.Cron); err != nil {
			return nil, err
		}
		queues = append(queues, q)
	}

	return queues, rows.Err()
}

func deleteQueue(db *sql.DB, workspacesDir string, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var workspaceID int64
	err = tx.QueryRow("SELECT workspace_id FROM queues WHERE id = ?", id).Scan(&workspaceID)
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM queues WHERE id = ?", id)
	if err != nil {
		return err
	}

	// Delete SQLite file for the queue
	queuePath := filepath.Join(workspacesDir, fmt.Sprintf("%d", workspaceID), "queues", fmt.Sprintf("%d.sqlite", id))
	if err := os.Remove(queuePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return tx.Commit()
}

var ErrNoMessages = errors.New("no messages in queue")

func emptyQueue(workspacesDir string, id int64, workspaceID int64) error {
	queuePath := filepath.Join(workspacesDir, fmt.Sprintf("%d", workspaceID), "queues", fmt.Sprintf("%d.sqlite", id))
	queueDB, err := sql.Open("sqlite3", queuePath)
	if err != nil {
		return err
	}
	defer queueDB.Close()

	q := goqite.New(goqite.NewOpts{
		DB:   queueDB,
		Name: fmt.Sprintf("queue_%d", id),
	})

	for {
		msg, err := q.Receive(context.Background())
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil // Queue is empty
			}
			return err
		}

		if err := q.Delete(context.Background(), msg.ID); err != nil {
			return err
		}
	}
}

func addSubscriber(db *sql.DB, queueID int64, webhookURL string) error {
	_, err := db.Exec("INSERT INTO subscribers (queue_id, webhook_url) VALUES (?, ?)", queueID, webhookURL)
	return err
}

func listSubscribers(db *sql.DB, queueID int64) ([]Subscriber, error) {
	rows, err := db.Query("SELECT id, queue_id, webhook_url FROM subscribers WHERE queue_id = ?", queueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subscribers []Subscriber
	for rows.Next() {
		var s Subscriber
		if err := rows.Scan(&s.ID, &s.QueueID, &s.WebhookURL); err != nil {
			return nil, err
		}
		subscribers = append(subscribers, s)
	}

	return subscribers, rows.Err()
}

func removeSubscriber(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM subscribers WHERE id = ?", id)
	return err
}

func sendMessage(workspacesDir string, id int64, workspaceID int64, message []byte) error {
	queuePath := filepath.Join(workspacesDir, fmt.Sprintf("%d", workspaceID), "queues", fmt.Sprintf("%d.sqlite", id))
	queueDB, err := sql.Open("sqlite3", queuePath)
	if err != nil {
		return err
	}
	defer queueDB.Close()

	q := goqite.New(goqite.NewOpts{
		DB:   queueDB,
		Name: fmt.Sprintf("queue_%d", id),
	})

	return q.Send(context.Background(), goqite.Message{Body: message})
}