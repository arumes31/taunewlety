#!/bin/sh

# Start Ollama in the background
ollama serve &

# Wait for Ollama to be ready
echo "Waiting for Ollama to start..."
until curl -s http://localhost:11434/api/tags > /dev/null; do
  sleep 2
done

# Pull the model specified in the environment or default to llama3.2:3b
MODEL=${OLLAMA_MODEL:-llama3.2:3b}
echo "Pulling model: $MODEL"
ollama pull $MODEL

# Keep the container running by waiting for the background process
wait
