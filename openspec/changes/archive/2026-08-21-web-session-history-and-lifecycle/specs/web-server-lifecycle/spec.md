## Purpose

Defines how the local `music-vault web` server shuts down and restarts cleanly, without requiring the process to be hard-killed, by handling OS signals and gracefully draining in-flight work.

## ADDED Requirements

### Requirement: Graceful shutdown on OS signals
The web server MUST listen for the OS interrupt (SIGINT) and terminate (SIGTERM) signals and, on receipt, shut the HTTP server down gracefully: stop accepting new connections, let in-flight requests complete within a bounded grace period, close the database handle, and exit with a clean status.

#### Scenario: Ctrl+C triggers graceful shutdown
- **WHEN** the web server receives SIGINT
- **THEN** it stops accepting new connections, finishes in-flight requests within the grace period, closes the database, and exits

#### Scenario: kill -TERM triggers graceful shutdown
- **WHEN** the web server receives SIGTERM
- **THEN** it performs the same graceful shutdown as for SIGINT

### Requirement: In-flight requests are allowed to complete
During shutdown the server MUST wait for currently running handlers (within a bounded grace period) before exiting, so that a request already being processed is not abruptly cut off.

#### Scenario: In-flight request allowed to finish
- **WHEN** a request is being handled while a shutdown signal arrives
- **THEN** the handler completes (within the grace period) before the process exits

### Requirement: Cleanup on shutdown
On shutdown the server MUST close the SQLite database handle and release resources before the process exits, so the database is not left in a transient state.

#### Scenario: Database closed on shutdown
- **WHEN** the server shuts down gracefully
- **THEN** the SQLite database handle is closed before the process exits

### Requirement: Graceful shutdown is non-fatal for normal operation
Graceful shutdown MUST be the normal way the server stops. A shutdown induced by a signal MUST NOT be reported as a fatal server error.

#### Scenario: Signal-driven stop is not an error
- **WHEN** the server exits because it received SIGINT or SIGTERM
- **THEN** the stop is not logged or treated as a fatal runtime failure
