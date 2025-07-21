package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

type Config struct {
	IncludedFiles []string `json:"includedFiles"`
	ExcludedFiles []string `json:"excludedFiles"`
	AI            AIConfig `json:"ai"`
	Severity      Severity `json:"severity"`
}

type AIConfig struct {
	APIEndpoint    string            `json:"apiEndpoint"`
	Headers        map[string]string `json:"headers"`
	PromptTemplate string            `json:"promptTemplate"`
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

	changedFiles, err := getChangedFiles(ctx, client, owner, repo, prNumber)
	if err != nil {
		fmt.Printf("Error getting changed files: %v\n", err)
		os.Exit(1)
	}

	filesToAnalyze, err := filterFiles(changedFiles, config)
	if err != nil {
		fmt.Printf("Error filtering files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files to analyze.\n", len(filesToAnalyze))

	var results []*FileAnalysisResult
	for _, file := range filesToAnalyze {
		analysis, err := analyzePatch(file.Patch, config, rules, aiAPIKey)
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
	content, err := ioutil.ReadFile(path)
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
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func getPullRequestNumber() (int, error) {
	githubEventPath := os.Getenv("GITHUB_EVENT_PATH")
	if githubEventPath == "" {
		return 0, fmt.Errorf("GITHUB_EVENT_PATH is not set")
	}
	eventData, err := ioutil.ReadFile(githubEventPath)
	if err != nil {
		return 0, err
	}
	var event struct {
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
	}
	err = json.Unmarshal(eventData, &event)
	if err != nil {
		return 0, err
	}
	return event.PullRequest.Number, nil
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
		changedFiles = append(changedFiles, &ChangedFile{
			Filename: *file.Filename,
			Patch:    *file.Patch,
		})
	}
	return changedFiles, nil
}

func filterFiles(files []*ChangedFile, config *Config) ([]*ChangedFile, error) {
	var filteredFiles []*ChangedFile
	for _, file := range files {
		included, err := matchAny(file.Filename, config.IncludedFiles)
		if err != nil {
			return nil, err
		}
		excluded, err := matchAny(file.Filename, config.ExcludedFiles)
		if err != nil {
			return nil, err
		}
		if included && !excluded {
			filteredFiles = append(filteredFiles, file)
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

func analyzePatch(patch string, config *Config, rules, apiKey string) (*AnalysisResult, error) {
	prompt := strings.Replace(config.AI.PromptTemplate, "{rules}", rules, 1)
	prompt = strings.Replace(prompt, "{code}", patch, 1)

	body := map[string]string{
		"prompt": prompt,
		"code":   patch,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", config.AI.APIEndpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	for key, value := range config.AI.Headers {
		value = strings.Replace(value, "{{AI_API_KEY}}", apiKey, -1)
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed: %s", resp.Status)
	}

	var result AnalysisResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
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