# A2A Multi-Agent Server

A streaming server implementation for the A2A protocol that processes text using OpenAI API. The server supports both streaming and non-streaming modes, with intelligent intent detection to route requests to appropriate AI assistants.

## Features

- Streaming and non-streaming text processing
- Intent detection for routing to different AI assistants
- Real-time progress updates
- Graceful shutdown handling
- Environment variable based configuration
- Comprehensive logging

## Prerequisites

- Go 1.16 or higher
- OpenAI API key

## Setup

1. Clone the repository:
```bash
git clone https://github.com/yourusername/a2a-multiagent-server.git
cd a2a-multiagent-server
```

2. Create a `.env` file in the project root:
```bash
# Create .env file
cp .env.example .env

# Edit the .env file with your actual values
# Required:
OPENAI_API_KEY=your-api-key-here

# Optional (these are the default values):
SERVER_HOST=localhost
SERVER_PORT=8080
OPENAI_MODEL=gpt-3.5-turbo
OPENAI_BASE_URL=https://api.openai.com/v1
```

3. Install dependencies:
```bash
go mod download
```

## Running the Server

Start the server:
```bash
go run main.go
```

The server will automatically load environment variables from your `.env` file. If the file is not found, it will use the default values except for `OPENAI_API_KEY` which is required.

## Environment Variables

- `OPENAI_API_KEY` (Required): Your OpenAI API key
- `SERVER_HOST` (Optional): Server host address (default: "localhost")
- `SERVER_PORT` (Optional): Server port (default: 8080)
- `OPENAI_MODEL` (Optional): OpenAI model to use (default: "gpt-3.5-turbo")
- `OPENAI_BASE_URL` (Optional): Base URL for API requests (default: "https://api.openai.com/v1")

## API Usage

The server implements the A2A protocol and supports the following features:

1. Text Processing:
   - Send text input to be processed by OpenAI
   - Receive streaming or non-streaming responses
   - Get real-time progress updates

2. Intent Detection:
   - Automatically detects whether the user wants to talk to XiaoMei or XiaoShuai
   - Routes the conversation to the appropriate AI assistant
   - Provides personalized responses based on the assistant's personality

## Architecture

The server uses a task-based architecture with the following components:

- `streamingTaskProcessor`: Handles the core processing logic
- `TaskManager`: Manages task lifecycle and state
- `A2AServer`: Implements the A2A protocol server

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Apache License Version 2.0 - see the LICENSE file for details.

## Acknowledgments

- [go-openai](https://github.com/sashabaranov/go-openai) for the OpenAI API client
- [trpc-go](https://github.com/trpc-group/trpc-go) for the A2A protocol implementation

## Docker Deployment

Build and run the server using Docker:

```bash
# Build the Docker image
docker build -t a2a-server .

# Run the container
docker run -p 8080:8080 --env-file .env a2a-server
```

## API Endpoints

The server exposes the A2A protocol endpoints for agent communication.

- `POST /`: Create a new task
- `GET /{taskID}`: Get task status
- `POST /{taskID}/cancel`: Cancel a task 