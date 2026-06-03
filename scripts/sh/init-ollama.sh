#!/bin/sh

# Start Ollama in the background
ollama serve &

# Wait for Ollama to be ready
echo "Waiting for Ollama to start..."
MAX_WAIT_SECS=60
counter=0
until curl -s http://localhost:11434/api/tags > /dev/null; do
  if [ $counter -ge $MAX_WAIT_SECS ]; then
    echo "Error: Ollama serve did not become ready within $MAX_WAIT_SECS seconds." >&2
    exit 1
  fi
  sleep 2
  counter=$((counter + 2))
done

# Pull the model specified in the environment or default to llama3.2:3b
MODEL=${OLLAMA_MODEL:-llama3.2:3b}
echo "Pulling model: $MODEL"
if ! ollama pull "$MODEL"; then
  echo "Error: Failed to pull model $MODEL" >&2
  exit 1
fi

# Keep the container running by waiting for the background process
wait
