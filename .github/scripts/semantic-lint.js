const simpleGit = require('simple-git');
const { Octokit } = require('@octokit/rest');
const fs = require('fs').promises;
const path = require('path');
const glob = require('glob');
const fetch = require('node-fetch');
const core = require('@actions/core');

// Get inputs from the action
const GITHUB_TOKEN = core.getInput('github-token', { required: true });
const AI_API_KEY = core.getInput('ai-api-key', { required: true });
const CONFIG_PATH = core.getInput('config-path') || '.github/semantic-lint.config.json';
const RULES_PATH = core.getInput('rules-path') || '.github/SemanticLintingRules.md';

// Initialize clients
const octokit = new Octokit({ auth: GITHUB_TOKEN });
const git = simpleGit();

async function loadConfig() {
    const configContent = await fs.readFile(CONFIG_PATH, 'utf8');
    return JSON.parse(configContent);
}

async function getChangedFiles() {
    const prNumber = github.context.payload.pull_request.number;
    const owner = github.context.repo.owner;
    const repo = github.context.repo.repo;

    const { data: files } = await octokit.pulls.listFiles({
        owner,
        repo,
        pull_number: prNumber,
    });

    return files.map(file => ({
        filename: file.filename,
        patch: file.patch
    }));
}

async function filterFiles(files, config) {
    return files.filter(file => {
        const shouldInclude = config.includedFiles.some(pattern =>
            glob.sync(pattern).includes(file.filename)
        );
        const shouldExclude = config.excludedFiles.some(pattern =>
            glob.sync(pattern).includes(file.filename)
        );
        return shouldInclude && !shouldExclude;
    });
}

async function readRulesFile() {
    try {
        const rulesContent = await fs.readFile(RULES_PATH, 'utf8');
        return rulesContent;
    } catch (error) {
        console.error(`Failed to read rules file ${RULES_PATH}:`, error);
        throw new Error('Rules file not found. Please create a rules file or specify a custom path in config.');
    }
}

async function analyzePatch(patch, config) {
    const headers = {};
    
    // Replace environment variables in headers
    Object.entries(config.ai.headers).forEach(([key, value]) => {
        headers[key] = value.replace(/\{\{AI_API_KEY\}\}/g, () => AI_API_KEY);
    });

    try {
        // Read rules from the repository
        const rules = await readRulesFile(config);

        // Replace placeholders in prompt template
        const prompt = config.ai.promptTemplate
            .replace('{rules}', rules)
            .replace('{code}', patch);

        const response = await fetch(config.ai.apiEndpoint, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...headers
            },
            body: JSON.stringify({
                prompt,
                code: patch
            })
        });

        if (!response.ok) {
            throw new Error(`API request failed: ${response.statusText}`);
        }

        const result = await response.json();
        return result;
    } catch (error) {
        console.error('Failed to analyze with AI API:', error);
        return { issues: [] };
    }
}

async function postResults(results, config) {
    const prNumber = github.context.payload.pull_request.number;
    const owner = github.context.repo.owner;
    const repo = github.context.repo.repo;

    let comment = '## Semantic Linting Results\n\n';
    
    results.forEach(({ filename, issues }) => {
        if (issues.length > 0) {
            comment += `### ${filename}\n\n`;
            issues.forEach(issue => {
                const severity = config.severity.error.includes(issue.type) ? '🔴' : '⚠️';
                comment += `${severity} **${issue.type}**: ${issue.message}\n`;
                if (issue.suggestion) {
                    comment += `> Suggestion: ${issue.suggestion}\n`;
                }
                comment += '\n';
            });
        }
    });

    await octokit.issues.createComment({
        owner,
        repo,
        issue_number: prNumber,
        body: comment
    });
}

async function main() {
    try {
        const config = await loadConfig();
        const changedFiles = await getChangedFiles();
        const filesToAnalyze = await filterFiles(changedFiles, config);

        const results = [];
        for (const file of filesToAnalyze) {
            const analysis = await analyzePatch(file.patch, config);
            results.push({
                filename: file.filename,
                issues: analysis.issues
            });
        }

        await postResults(results, config);

        // Exit with error if there are any error-level issues
        const hasErrors = results.some(result =>
            result.issues.some(issue =>
                config.severity.error.includes(issue.type)
            )
        );

        process.exit(hasErrors ? 1 : 0);
    } catch (error) {
        console.error('Error running semantic linter:', error);
        process.exit(1);
    }
}

main();
