# Cinemesis

Cinemesis is a **Go-based RESTful API** designed as a personal project to showcase modern Go development practices, including API design, routing, and robust data handling with authentication and email features. It provides a comprehensive set of endpoints for managing movie information, user accounts, and token-based authentication. This project serves as an excellent example of a well-structured Go application.

##

**Created with the help of the wonderful book "Let's Go Further" by Alex Edwards.**

##

### Installation and Running

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/sinqx/cinemesis.git
    cd cinemesis
    ```

2.  **Environment Variables:**

    Create a `.env` file in the root of the project. This file will hold your application's environment variables. These are crucial for configuring database connections, email sending, and other services.

    ```
    PORT=8080
    POSTGRESQL_CONN="postgres://user:password@host:port/database_name?sslmode=disable"
    SMTP_USERNAME=your_smtp_username
    SMTP_PASSWORD=your_smtp_password
    SMTP_SENDER=sender@example.com
    ```

    - **`PORT`**: The port on which the API server will listen (e.g., `8080`).
    - **`POSTGRESQL_CONN`**: The connection string for your PostgreSQL database. **Remember to replace `user`, `password`, `host`, `port`, and `database_name` with your actual database credentials.** For local development, `sslmode=disable` is often sufficient.
    - **`SMTP_USERNAME`**: The username for your SMTP server, used for sending emails (e.g., password resets, notifications).
    - **`SMTP_PASSWORD`**: The password for your SMTP server.
    - **`SMTP_SENDER`**: The email address from which automated emails will be sent.

3.  **Run the application using `make`:**

    The `Makefile` simplifies the build and run process. To start the API server:

    ```bash
    make run/api
    ```

    This command will compile the application and start the API server, typically on `http://localhost:8080`.

    If you want to run with hot-reloading (requires `air` to be installed globally or via Go modules), use:

    ```bash
    make run/air
    ```

---

## 📂 Project Structure

The project follows a clean and logical structure, emphasizing separation of concerns and maintainability:

```
cinemesis/

├── cmd/
│   └── api/                    # Main API application entry point
│       ├── context.go          # Request context utilities
│       ├── errors.go           # Custom error definitions
│       ├── healthcheck.go      # Health check handler
│       ├── helpers.go          # Utility functions
│       ├── main.go             # Main application logic
│       ├── middleware.go       # HTTP middleware definitions
│       ├── movies.go           # Movie related handlers
│       ├── routes.go           # API route definitions
│       ├── server.go           # HTTP server setup
│       ├── tokens.go           # Token related handlers
│       └── users.go            # User related handlers
├── internal/                   # Internal packages not exposed externally
│   ├── data/                   # Data access layer (models & database operations)
│   │   ├── filters.go
│   │   ├── models.go
│   │   ├── movies.go
│   │   ├── permissions.go
│   │   ├── runtime.go
│   │   ├── tokens.go
│   │   └── users.go
│   ├── mailer/                 # Email sending functionalities
│   │   ├── templates/          # Email templates
│   │   │   ├── token_activation.tmpl
│   │   │   ├── token_password_reset.tmpl
│   │   │   └── user_welcome.tmpl
│   │   └── mailer.go
│   ├── validator/              # Input validation utilities
│   │   └── validator.go
│   └── vcs.go                  # Version control system integration
├── migrations/                 # Database migration scripts
│
├── .air.toml                   # Configuration for `air` (live-reloading tool)
├── .env                        # Environment variables for local development
├── go.mod                      # Go modules dependency definition
├── go.sum                      # Go modules checksums and versions
├── Makefile                    # Automation commands for building, running, etc.
└── README.md                   # Project documentation
```

---

## 🛠️ Make Commands

The `Makefile` provides several convenient commands for development and operations:

- **`make help`**: Prints this help message with all available commands.
- **`make run/api`**: Compiles and runs the main API application (`cmd/api`). This is the standard way to start the server.
- **`make run/build`**: Builds the `cmd/api` application, creating an executable binary.
- **`make run/air`**: Builds and runs the `cmd/api` application using `air` for automatic hot-reloading on code changes. Useful for rapid development.
- **`make db/psql`**: Connects to the PostgreSQL database using the `psql` client (requires `psql` to be installed and configured with `POSTGRESQL_CONN`).
- **`make db/migrations/new name=your_migration_name`**: Creates a new database migration file with the specified name. Replace `your_migration_name` with a descriptive name.
- **`make db/migrations/up`**: Applies all pending "up" database migrations to the connected database.
- **`make tidy`**: Tidies module dependencies and formats all `.go` files according to Go standards.
- **`make audit`**: Runs quality control checks on the codebase (e.g., linting, static analysis).

---

## 📌 API Endpoints

The Cinemesis API exposes the following versioned (`/v1/`) endpoints:

| Method   | Endpoint                    | Description                                              | Permissions Required        |
| :------- | :-------------------------- | :------------------------------------------------------- | :-------------------------- |
| `GET`    | `/v1/healthcheck`           | Simple health check endpoint.                            | None                        |
| `POST`   | `/v1/movies`                | Adds a new movie to the collection.                      | `movies:write`              |
| `GET`    | `/v1/movies`                | Retrieves a list of all movies.                          | `movies:read`               |
| `GET`    | `/v1/movies/:id`            | Retrieves a single movie by its unique ID.               | `movies:read`               |
| `PATCH`  | `/v1/movies/:id`            | Updates an existing movie identified by its ID.          | `movies:write`              |
| `DELETE` | `/v1/movies/:id`            | Deletes a movie by its unique ID.                        | `movies:write`              |
| `POST`   | `/v1/users`                 | Registers a new user.                                    | None                        |
| `PUT`    | `/v1/users/activated`       | Activates a user account using an activation token.      | None                        |
| `POST`   | `/v1/tokens/reset`          | Creates a password reset token for a user.               | None                        |
| `POST`   | `/v1/tokens/authentication` | Authenticates a user and issues an authentication token. | None                        |
| `POST`   | `/v1/tokens/activation`     | (Re)generates an activation token for a user.            | None                        |
| `GET`    | `/debug/vars`               | Exposes expvar metrics for application monitoring.       | None (typically restricted) |

### Request & Response Examples (Conceptual)

#### `POST /v1/movies` Example

**Request Body:**

```json
{
  "title": "Inception",
  "director": "Christopher Nolan",
  "year": 2010
}
```

#### `GET /v1/movies` Example

**Response Body:**

```json
[
  {
    "id": "1",
    "title": "Inception",
    "director": "Christopher Nolan",
    "year": 2010
  },
  {
    "id": "2",
    "title": "The Matrix",
    "director": "The Wachowskis",
    "year": 1999
  }
]
```

---
