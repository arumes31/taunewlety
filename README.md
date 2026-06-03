<p align="center">
  <img src="logo.svg" alt="TauNewlety Logo" width="240">
</p>

# TauNewlety
**The Modern AI Newsletter for your Plex Server**


TauNewlety is a Go-based application that combines the power of Tautulli and local LLMs (Ollama) to generate beautiful, personalized daily newsletters for your Plex users.

## Features
- **AI-Generated Newsletters**: Uses **Llama 3.2 (3B)** via local Ollama, optimized for CPU performance.
- **Smart Filtering**: 
    - Excludes items already recommended in the last 7 days.
    - Excludes items already watched by more than 4 users.
- **Modern WebGUI**: Manage all settings and preview newsletters before they go out.
- **Docker Ready**: All-in-one setup with Docker Compose.
- **Security First**: 
    - Secure login via environment variables.
    - GHCR image building with security scanning.

## Quick Start
1. Clone the repository.
2. Configure your `.env` file:
    ```env
    APP_USER=admin
    APP_PASS=yourpassword
    NOTIFY_EMAIL=users@example.com
    ```
3. Run with Docker Compose:
    ```bash
    docker-compose up -d
    ```
4. Access the WebGUI at `http://localhost:8080`.

## Configuration
All configuration (Tautulli URL, API Keys, SMTP, Ollama settings) can be managed directly in the WebGUI. Settings are persistent across reboots.

## Tech Stack
- **Language**: Go
- **Database**: SQLite (GORM)
- **Web Framework**: Gin
- **LLM**: Ollama (Local)
- **Email**: SMTP
