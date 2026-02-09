#!/bin/bash

# retry.sh

CMD="$INPUT_COMMAND"
MAX="$INPUT_MAX_ATTEMPTS"
INIT_DELAY="$INPUT_INITIAL_DELAY"
REGEX="$INPUT_RETRY_ON_REGEX"

for ((i=1; i<=MAX; i++)); do
  echo "Attempt $i..."

  # Run command, capture output
  if output=$(eval "$CMD" 2>&1); then
    printf "%s\n" "$output"
    exit 0
  fi

  # Always log the output of a failed attempt
  printf "%s\n" "$output"

  # Exit on max attempts
  if (( i == MAX )); then
    echo "Error: Max attempts reached."
    exit 1
  fi

   # Exit on non-retryable error
  if [[ ! "$output" =~ $REGEX ]]; then
    echo "Error: Failure did not match retry pattern."
    exit 1
  fi

  # Calculate randomized exponential backoff
  min_delay=$(( INIT_DELAY * (2 ** (i-1)) ))
  jittered_increment=$(( RANDOM % (min_delay + 1) ))

  sleep $(( min_delay + jittered_increment ))
done
