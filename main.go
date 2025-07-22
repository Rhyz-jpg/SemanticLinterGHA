package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type Config struct {
	IncludedFiles []string `json:"includedFiles"`
	ExcludedFiles []string `json:"excludedFiles"`
	AI            AIConfig `json:"ai"`
	Severity      Severity `json:"severity"`
}

type AIConfig struct {
	PromptTemplate string            `json:"promptTemplate"`
	Gemini         GeminiConfig      `json:"gemini"`
}

type GeminiConfig struct {
	Model string `json:"model"`
}


type Severity struct {
	Error   []string `json:"error"`
	Warning []string `json:"warning"`
}

type ChangedFile struct {
	Filename string
	Patch    string
}

type AnalysisResult struct {
	Issues []Issue `json:"issues"`
}

type Issue struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type FileAnalysisResult struct {
	Filename string
	Issues   []Issue
}

type LLMProvider interface {
	Analyze(patch, prompt, apiKey string) (*AnalysisResult, error)
}

type GeminiProvider struct {
	Config GeminiConfig
}


func (p *GeminiProvider) Analyze(patch, prompt, apiKey string) (*AnalysisResult, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(p.Config.Model)
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content found in gemini response")
	}

	part := resp.Candidates[0].Content.Parts[0]
	text, ok := part.(genai.Text)
	if !ok {
		return nil, fmt.Errorf("unexpected part type: %T", part)
	}

	jsonString := string(text)
	jsonString = strings.TrimPrefix(jsonString, "```json")
	jsonString = strings.TrimSuffix(jsonString, "```")
	jsonString = strings.TrimSpace(jsonString)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(jsonString), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analysis result from gemini response: %w", err)
	}

	return &result, nil
}


func main() {
	fmt.Println("Starting semantic linter...")

	githubToken := os.Getenv("INPUT_GITHUB-TOKEN")
	if githubToken == "" {
		fmt.Println("GitHub token is not set.")
		os.Exit(1)
	}

	aiAPIKey := os.Getenv("INPUT_AI-API-KEY")
	if aiAPIKey == "" {
		fmt.Println("AI API key is not set.")
		os.Exit(1)
	}

	configPath := os.Getenv("INPUT_CONFIG-PATH")
	if configPath == "" {
		configPath = ".github/semantic-lint.config.json"
	}

	rulesPath := os.Getenv("INPUT_RULES-PATH")
	if rulesPath == "" {
		rulesPath = ".github/SemanticLintingRules.md"
	}

	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	rules, err := readRulesFile(rulesPath)
	if err != nil {
		fmt.Printf("Error reading rules file: %v\n", err)
		os.Exit(1)
	}

	provider := &GeminiProvider{Config: config.AI.Gemini}

	fmt.Println("Config and rules loaded successfully.")

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	prNumber, err := getPullRequestNumber()
	if err != nil {
		fmt.Printf("Error getting pull request number: %v\n", err)
		os.Exit(1)
	}

	owner, repo := getRepoInfo()

	fmt.Printf("Fetching changed files for PR #%d in %s/%s\n", prNumber, owner, repo)

	changedFiles, err := getChangedFiles(ctx, client, owner, repo, prNumber)
	if err != nil {
		fmt.Printf("Error getting changed files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d raw changed files.\n", len(changedFiles))

	filesToAnalyze, err := filterFiles(changedFiles, config)
	if err != nil {
		fmt.Printf("Error filtering files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files to analyze.\n", len(filesToAnalyze))

	var results []*FileAnalysisResult
	for _, file := range filesToAnalyze {
		analysis, err := analyzePatch(file.Patch, config, rules, aiAPIKey, provider)
		if err != nil {
			fmt.Printf("Error analyzing patch for %s: %v\n", file.Filename, err)
			continue
		}
		results = append(results, &FileAnalysisResult{
			Filename: file.Filename,
			Issues:   analysis.Issues,
		})
	}

	err = postResults(ctx, client, owner, repo, prNumber, results, config)
	if err != nil {
		fmt.Printf("Error posting results: %v\n", err)
		os.Exit(1)
	}

	if hasErrors(results, config) {
		os.Exit(1)
	}
}

func loadConfig(path string) (*Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config Config
	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func readRulesFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func getRepoInfo() (string, string) {
	repoSlug := os.Getenv("GITHUB_REPOSITORY")
	parts := strings.Split(repoSlug, "/")
	return parts[0], parts[1]
}

func getChangedFiles(ctx context.Context, client *github.Client, owner, repo string, prNumber int) ([]*ChangedFile, error) {
	files, _, err := client.PullRequests.ListFiles(ctx, owner, repo, prNumber, nil)
	if err != nil {
		return nil, err
	}

	var changedFiles []*ChangedFile
	for _, file := range files {
		if file.Filename != nil && file.Patch != nil {
			changedFiles = append(changedFiles, &ChangedFile{
				Filename: *file.Filename,
				Patch:    *file.Patch,
			})
		}
	}
	return changedFiles, nil
}

func filterFiles(files []*ChangedFile, config *Config) ([]*ChangedFile, error) {
	var filteredFiles []*ChangedFile
	fmt.Printf("Filtering files with patterns: included=%v, excluded=%v\n", config.IncludedFiles, config.ExcludedFiles)
	for _, file := range files {
		fmt.Printf("Checking file: %s\n", file.Filename)
		included, err := matchAny(file.Filename, config.IncludedFiles)
		if err != nil {
			return nil, err
		}
		excluded, err := matchAny(file.Filename, config.ExcludedFiles)
		if err != nil {
			return nil, err
		}
		if included && !excluded {
			fmt.Printf("  -> Included\n")
			filteredFiles = append(filteredFiles, file)
		} else {
			fmt.Printf("  -> Excluded (included=%v, excluded=%v)\n", included, excluded)
		}
	}
	return filteredFiles, nil
}

func matchAny(path string, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		match, err := doublestar.Match(pattern, path)
		if err != nil {
			return false, err
		}
		if match {
			return true, nil
		}
	}
	return false, nil
}

func analyzePatch(patch string, config *Config, rules, apiKey string, provider LLMProvider) (*AnalysisResult, error) {
	prompt := strings.Replace(config.AI.PromptTemplate, "{rules}", rules, 1)
	prompt = strings.Replace(prompt, "{code}", patch, 1)

	return provider.Analyze(patch, prompt, apiKey)
}

func postResults(ctx context.Context, client *github.Client, owner, repo string, prNumber int, results []*FileAnalysisResult, config *Config) error {
	var comment strings.Builder
	comment.WriteString("## Semantic Linting Results\n\n")

	for _, result := range results {
		if len(result.Issues) > 0 {
			comment.WriteString(fmt.Sprintf("### %s\n\n", result.Filename))
			for _, issue := range result.Issues {
				severityIcon := "⚠️"
				for _, errorType := range config.Severity.Error {
					if issue.Type == errorType {
						severityIcon = "🔴"
						break
					}
				}
				comment.WriteString(fmt.Sprintf("%s **%s**: %s\n", severityIcon, issue.Type, issue.Message))
				if issue.Suggestion != "" {
					comment.WriteString(fmt.Sprintf("> Suggestion: %s\n", issue.Suggestion))
				}
				comment.WriteString("\n")
			}
		}
	}

	commentString := comment.String()
	_, _, err := client.Issues.CreateComment(ctx, owner, repo, prNumber, &github.IssueComment{
		Body: &commentString,
	})
	return err
}

func hasErrors(results []*FileAnalysisResult, config *Config) bool {
	for _, result := range results {
		for _, issue := range result.Issues {
			for _, errorType := range config.Severity.Error {
				if issue.Type == errorType {
					return true
				}
			}
		}
	}
	return false
}

func getPullRequestNumber() (int, error) {
	prNumberStr := os.Getenv("INPUT_PR-NUMBER")
	if prNumberStr != "" {
		prNumber, err := strconv.Atoi(prNumberStr)
		if err == nil {
			return prNumber, nil
		}
	}

	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return 0, fmt.Errorf("GITHUB_EVENT_PATH is not set")
	}

	data, err := os.ReadFile(eventPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read event file: %w", err)
	}

	var payload struct {
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, fmt.Errorf("failed to unmarshal event payload: %w", err)
	}

	if payload.PullRequest.Number == 0 {
		return 0, fmt.Errorf("pull request number not found in event payload")
	}

	return payload.PullRequest.Number, nil
}