# This script builds and runs the semantic-linter Docker container locally.

# --- Configuration ---
# Load environment variables from .env file
if (Test-Path .\.env) {
    Get-Content .\.env | ForEach-Object {
        if ($_ -match "^(.*?)=(.*)$") {
            $name = $matches[1]
            $value = $matches[2]
            [System.Environment]::SetEnvironmentVariable($name, $value)
        }
    }
}

$GITHUB_TOKEN = $env:GITHUB_TOKEN
$AI_API_KEY = $env:GEMINI_API_KEY
$REPO_SLUG = "Rhyz-jpg/DummyAPi"
$PR_NUMBER = "1"


# --- Build and Run ---
Write-Host "Building the Docker image..."
docker build -t semantic-linter .

Write-Host "Running the linter..."
docker run --rm `
  -e INPUT_GITHUB-TOKEN=$GITHUB_TOKEN `
  -e INPUT_AI-API-KEY=$AI_API_KEY `
  -e GITHUB_REPOSITORY=$REPO_SLUG `
  -e INPUT_PR-NUMBER=$PR_NUMBER `
  -v "${PWD}:/app" `
  semantic-linter