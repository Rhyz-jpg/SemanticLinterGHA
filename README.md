# SemanticLinterGHA

Semantic linter using AI for GitHub Actions

## Running Locally

You can run the Semantic Linter locally using Docker. This is useful for testing your configuration and rules before committing them.

### Prerequisites

- Docker installed and running.
- A GitHub Personal Access Token with `repo` scope.
- An AI provider API key (e.g., Gemini).

### 1. Build the Docker Image

Build the Docker image from the root of the repository:

```sh
docker build -t semantic-linter .
```

### 2. Run the Linter

To run the linter, you need to provide several environment variables to the Docker container.

- `INPUT_GITHUB-TOKEN`: Your GitHub Personal Access Token.
- `INPUT_AI-API-KEY`: Your AI provider API key.
- `GITHUB_REPOSITORY`: The slug of the repository you want to lint (e.g., `your-username/your-repo`).
- `INPUT_PR-NUMBER`: The number of the pull request to lint.

You will also need to mount your local repository into the container so the linter can access the files.

Here is an example command:

```sh
docker run --rm \
  -e INPUT_GITHUB-TOKEN="your_github_token" \
  -e INPUT_AI-API-KEY="your_ai_api_key" \
  -e GITHUB_REPOSITORY="your_username/your_repo" \
  -e INPUT_PR-NUMBER="1" \
  -v "$(pwd)":/app \
  semantic-linter
```

**Note:** The `-v "$(pwd)":/app` command mounts your current working directory into the `/app` directory in the container. This allows the linter to see your local files. On Windows, you may need to use a different syntax for the volume mount, like `-v ${PWD}:/app` in PowerShell or `-v %cd%:/app` in Command Prompt.

### Using the PowerShell Script (Windows)

For Windows users, a PowerShell script is provided to simplify the process.

1.  **Edit `run-locally.ps1`**: Open the [`run-locally.ps1`](run-locally.ps1) script and replace the placeholder values for `$env:INPUT_GITHUB_TOKEN`, `$env:INPUT_AI_API_KEY`, `$env:GITHUB_REPOSITORY`, and `$env:INPUT_PR_NUMBER` with your actual data.

2.  **Run the script**: Execute the script from your PowerShell terminal:

    ```powershell
    .\run-locally.ps1
    ```