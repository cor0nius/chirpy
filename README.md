# Chirpy - A Backend Web Server in Go

Chirpy is a backend HTTP server for a simple Twitter-like application, built entirely in Go. It provides a robust JSON API for user management, posting "chirps", and authentication, all while interacting with a PostgreSQL database.

This project was completed as part of the ["Learn HTTP Servers in Go"](https://www.boot.dev/courses/learn-http-servers-golang) course on boot.dev, focusing on building a production-ready API from the ground up.

## Features

*   **RESTful API:** A complete JSON API for creating, reading, and deleting "chirps".
*   **User Authentication:** Secure user creation, login, and profile updates with password hashing (`bcrypt`) and JSON Web Tokens (JWTs).
*   **Token Management:** Implements both short-lived access tokens and long-lived refresh tokens for persistent sessions.
*   **Authorization:** Protected endpoints ensure that users can only delete their own chirps.
*   **Database Integration:** Connects to a PostgreSQL database, with `sqlc` used to generate type-safe Go code from raw SQL queries.
*   **Webhook Integration:** Includes an endpoint to securely process webhooks from the external "Polka" service for user upgrades, authenticated with API keys.
*   **Middleware:** Custom middleware is used for server metrics and logging.
*   **Admin Endpoints:** Provides administrative endpoints for health checks and monitoring server traffic.

## Key Concepts & Skills Learned

*   **HTTP Server Development:** Built a web server from scratch using only Go's standard `net/http` library, without relying on a major framework.
*   **API & Routing Design:** Structured a clean and logical API with distinct routes for different resources (`/users`, `/chirps`, `/login`, etc.).
*   **Database Management with `sqlc`:**
    *   Wrote SQL schema migrations and queries for a PostgreSQL database.
    *   Utilized `sqlc` to generate type-safe, boilerplate-free Go code for all database operations, ensuring a clean separation of concerns.
*   **Authentication & Security:**
    *   Implemented a full authentication flow using JWTs for access and refresh tokens.
    *   Secured user passwords by hashing them with `bcrypt`.
    *   Managed authorization by validating tokens and checking resource ownership before allowing modifications.
*   **Configuration Management:** Handled sensitive information like JWT secrets and database URLs securely using environment variables.
*   **JSON Data Handling:** Proficiently encoded and decoded JSON data for API requests and responses.
