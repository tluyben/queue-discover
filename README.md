# 📬 Queue Discovery Service

This Go-based service manages SQLite-backed queues within workspaces, providing REST APIs for queue, subscriber, and message management. It's built using goqite for persistent message queues.

## 🌟 Features

- Queue management (create, list, delete, empty)
- Subscriber management (add, list, remove)
- Message sending and processing
- Workspace-based SQLite file management
- Automatic message delivery to subscribers via webhooks

## 🛠 Prerequisites

- Go 1.16 or higher
- SQLite3

## 🚀 Installation

1. Clone the repository:

   ```
   git clone https://github.com/yourusername/queue-discover.git
   cd queue-discover
   ```

2. Install dependencies:
   ```
   go get github.com/gorilla/mux
   go get github.com/mattn/go-sqlite3
   go get github.com/maragudk/goqite
   ```

## 🏃‍♂️ Usage

1. Build the service:

   ```
   make build
   ```

2. Run the service:
   ```
   make run
   ```

The service will start on port 8080 by default. You can specify a different port using the `-port` flag:

```
./queue-discover -port 9000
```

## 🔧 API Endpoints

### Queues

- Create a new queue:

  ```
  curl -X POST http://localhost:8080/queues \
    -H "Content-Type: application/json" \
    -d '{"workspace_id": 1, "name": "my-queue", "description": "My first queue", "cron": "*/5 * * * *"}'
  ```

- List all queues in a workspace:

  ```
  curl "http://localhost:8080/queues?workspace_id=1"
  ```

- Delete a queue:

  ```
  curl -X DELETE http://localhost:8080/queues/1
  ```

- Empty a queue:
  ```
  curl -X POST http://localhost:8080/queues/1/empty
  ```

### Subscribers

- Add a subscriber to a queue:

  ```
  curl -X POST http://localhost:8080/queues/1/subscribers \
    -H "Content-Type: application/json" \
    -d '{"webhook_url": "https://example.com/webhook"}'
  ```

- List subscribers for a queue:

  ```
  curl http://localhost:8080/queues/1/subscribers
  ```

- Remove a subscriber:
  ```
  curl -X DELETE http://localhost:8080/queues/1/subscribers/1
  ```

### Messages

- Send a message to a queue:
  ```
  curl -X POST http://localhost:8080/queues/1/messages \
    -H "Content-Type: application/json" \
    -d '{"content": "Hello, World!"}'
  ```

## 🧑‍💻 Development

To run the service in development mode with automatic reloading:

```
make dev
```

## 🧪 Testing

Run the test suite:

```
make test
```

## 🧹 Cleaning up

Remove the built binary:

```
make clean
```

## 📚 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
