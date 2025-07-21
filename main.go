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
	Provider       string            `json:"provider"`
	PromptTemplate string            `json:"promptTemplate"`
	Gemini         GeminiConfig      `json:"gemini"`
	OpenAI         OpenAIConfig      `json:"openai"`
	Anthropic      AnthropicConfig   `json:"anthropic"`
}

type GeminiConfig struct {
	APIEndpoint string            `json:"apiEndpoint"`
	Headers     map[string]string `json:"headers"`
}

type OpenAIConfig struct {
	APIEndpoint string            `json:"apiEndpoint"`
	Model       string            `json:"model"`
	Headers     map[string]string `json:"headers"`
}

type AnthropicConfig struct {
	APIEndpoint string            `json:"apiEndpoint"`
	Model       string            `json:"model"`
	Headers     map[string]string `json:"headers"`
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

type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

type LLMProvider interface {
	Analyze(patch, prompt, apiKey string) (*AnalysisResult, error)
}

type GeminiProvider struct {
	Config GeminiConfig
}

type OpenAIProvider struct {
	Config OpenAIConfig
}

type AnthropicProvider struct {
	Config AnthropicConfig
}

func (p *GeminiProvider) Analyze(patch, prompt, apiKey string) (*AnalysisResult, error) {
	geminiReq := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{
						Text: prompt,
					},
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	endpoint := strings.Replace(p.Config.APIEndpoint, "{{AI_API_KEY}}", apiKey, -1)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range p.Config.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %s: %s", resp.Status, string(body))
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content found in gemini response")
	}

	jsonString := geminiResp.Candidates[0].Content.Parts[0].Text
	jsonString = strings.TrimPrefix(jsonString, "```json")
	jsonString = strings.TrimSuffix(jsonString, "```")
	jsonString = strings.TrimSpace(jsonString)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(jsonString), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analysis result from gemini response: %w", err)
	}

	return &result, nil
}

type OpenAIRequest struct {
	Model    string           `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *OpenAIProvider) Analyze(patch, prompt, apiKey string) (*AnalysisResult, error) {
	openAIReq := OpenAIRequest{
		Model: p.Config.Model,
		Messages: []OpenAIMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	bodyBytes, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", p.Config.APIEndpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range p.Config.Headers {
		value = strings.Replace(value, "{{AI_API_KEY}}", apiKey, -1)
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %s: %s", resp.Status, string(body))
	}

	var openAIResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode openai response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices found in openai response")
	}

	jsonString := openAIResp.Choices[0].Message.Content
	jsonString = strings.TrimPrefix(jsonString, "```json")
	jsonString = strings.TrimSuffix(jsonString, "```")
	jsonString = strings.TrimSpace(jsonString)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(jsonString), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analysis result from openai response: %w", err)
	}

	return &result, nil
}

type AnthropicRequest struct {
	Model    string             `json:"model"`
	Messages []AnthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (p *AnthropicProvider) Analyze(patch, prompt, apiKey string) (*AnalysisResult, error) {
	anthropicReq := AnthropicRequest{
		Model: p.Config.Model,
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens: 4096,
	}

	bodyBytes, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", p.Config.APIEndpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range p.Config.Headers {
		value = strings.Replace(value, "{{AI_API_KEY}}", apiKey, -1)
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %s: %s", resp.Status, string(body))
	}

	var anthropicResp AnthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to decode anthropic response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return nil, fmt.Errorf("no content found in anthropic response")
	}

	jsonString := anthropicResp.Content[0].Text
	jsonString = strings.TrimPrefix(jsonString, "```json")
	jsonString = strings.TrimSuffix(jsonString, "```")
	jsonString = strings.TrimSpace(jsonString)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(jsonString), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analysis result from anthropic response: %w", err)
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

	var provider LLMProvider
	switch config.AI.Provider {
	case "gemini":
		provider = &GeminiProvider{Config: config.AI.Gemini}
	case "openai":
		provider = &OpenAIProvider{Config: config.AI.OpenAI}
	case "anthropic":
		provider = &AnthropicProvider{Config: config.AI.Anthropic}
	default:
		fmt.Printf("Unsupported AI provider: %s\n", config.AI.Provider)
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