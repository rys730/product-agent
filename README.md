# Product Agent

A self-hosted service that listens for GitHub issue webhooks, retrieves relevant
code context, calls an LLM, and posts a structured engineering specification as
a GitHub comment.

---

## How it works

```
GitHub Issue (webhook)
        │
        ▼
  Contains "product-agent:comment"?  ── no ──▶ skip
        │ yes
        ▼
  Extract keywords from title + body
        │
        ▼
  Retrieve top 5 relevant files
  (local disk scan  OR  GitHub API)
        │
        ▼
  Build prompt:
    README + PRODUCT-AGENT.md + repo tree + code snippets + issue
        │
        ▼
  Call LLM (OpenAI / OpenRouter / any compatible API)
        │
        ▼
  Parse JSON response → structured spec
        │
        ▼
  Post Markdown comment on the GitHub issue
```

---

## Prerequisites

- Go 1.22+
- A GitHub personal access token (PAT) with `repo` scope
- An OpenAI API key **or** an [OpenRouter](https://openrouter.ai) key
- [`cloudflared`](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/) or `ngrok` to expose your local server to GitHub webhooks

---

## Quick start

### 1. Clone and build

```bash
git clone https://github.com/rys730/product-agent.git
cd product-agent
go build -o product-agent .
```

### 2. Configure environment

Copy the example env file and fill in your values:

```bash
cp .env.example .env
```

Edit `.env`:

```bash
# Required
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx
OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxx

# Optional — defaults shown
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-4o
PORT=8080
RETRIEVE_MODE=github          # "local" or "github"
WEBHOOK_ACTIONS=opened,edited
REPO_EXTENSIONS=.go,.ts,.js,.py,.java,.rs,.rb,.cs,.cpp,.c
```

### 3. Run the server

```bash
./product-agent
```

```
[product-agent] starting server on :8080
[product-agent] retrieve mode: github
[product-agent] webhook actions: map[edited:true opened:true]
```

### 4. Expose to GitHub webhooks

Using **cloudflared** (free, no account needed):

```bash
cloudflared tunnel --url http://localhost:8080
```

Using **ngrok**:

```bash
ngrok http 8080
```

Copy the generated HTTPS URL (e.g. `https://abc123.trycloudflare.com`).

### 5. Register the webhook on GitHub

1. Go to your repo → **Settings → Webhooks → Add webhook**
2. **Payload URL:** `https://abc123.trycloudflare.com/webhook`
3. **Content type:** `application/json`
4. **Events:** select **Issues** only
5. Click **Add webhook**

---

## Triggering the agent

The agent only processes issues that contain the trigger phrase anywhere in the body:

```
product-agent:comment
```

**Example issue body:**

```
We need to add rate limiting to the API handler.
Should support per-IP limits and return 429 with Retry-After.
Must be configurable via env vars.

product-agent:comment
```

The agent will post a structured spec comment on that issue.

---

## Retrieve modes

### `RETRIEVE_MODE=github` (recommended for most setups)

Uses the GitHub Trees + Contents API. No local clone needed. The agent fetches
the repository file tree, scores files by keyword frequency in their content,
and retrieves the top 5 most relevant files.

```bash
RETRIEVE_MODE=github
# REPO_BRANCH=main   # optional, defaults to the repo's default branch
```

### `RETRIEVE_MODE=local`

Scans a locally cloned repository on disk. Faster for large repos since no
API calls are made for file content.

```bash
RETRIEVE_MODE=local
REPO_PATH=/path/to/your/local/repo/clone
```

---

## Codebase index — `PRODUCT-AGENT.md`

Place a `PRODUCT-AGENT.md` file in the **root of your target repository** to
give the agent a map of your codebase. The agent reads it automatically in
both local and github modes.

```markdown
# Codebase Index

## internal/auth/handler.go
- `Login(w, r)` — validates credentials, issues JWT
- `Logout(w, r)` — invalidates the session token

## internal/billing/processor.go
- `ChargeCard(userID string, amount int) error` — calls Stripe, records transaction
- `Refund(transactionID string) error` — issues partial or full refund
```

If `PRODUCT-AGENT.md` is absent, the agent falls back to an auto-generated
index derived from function signatures in the retrieved files.

> **Generating `PRODUCT-AGENT.md`:** ask Claude or another LLM to scan your
> codebase and generate the file using the prompt template below.

### Prompt to generate `PRODUCT-AGENT.md`

Copy this prompt and paste it into Claude, ChatGPT, or any LLM with access
to your codebase files:

```
You are a senior software engineer. Scan this codebase and generate a
PRODUCT-AGENT.md file.

For each source file, list all exported AND important unexported functions
with their full signature and a one-line description of what they do.
Mention key side effects (e.g. "calls Stripe API", "writes to PostgreSQL").

Output format — follow EXACTLY:

# Codebase Index

## path/to/file.go
- `FunctionName(param type) returnType` — what it does in one line

Rules:
- One line per function, no paragraphs
- File paths relative to repo root
- Skip files with no meaningful functions (pure types/constants)
- Output ONLY the markdown, no preamble
```

---

## Using OpenRouter

To use any model via [OpenRouter](https://openrouter.ai):

```bash
OPENAI_API_KEY=sk-or-xxxxxxxxxxxxxxxxxxxx
OPENAI_BASE_URL=https://openrouter.ai/api/v1
OPENAI_MODEL=anthropic/claude-sonnet-4-5
OPENROUTER_REFERER=https://github.com/your-org/your-repo
OPENROUTER_TITLE=product-agent
```

---

## Configuration reference

| Variable             | Required | Default                      | Description                                         |
|----------------------|----------|------------------------------|-----------------------------------------------------|
| `GITHUB_TOKEN`       | ✅        | —                            | GitHub PAT with `repo` scope                        |
| `OPENAI_API_KEY`     | ✅        | —                            | OpenAI or OpenRouter API key                        |
| `OPENAI_BASE_URL`    | ❌        | `https://api.openai.com/v1` | Base URL for any OpenAI-compatible API              |
| `OPENAI_MODEL`       | ❌        | `gpt-4o`                     | Model name                                          |
| `PORT`               | ❌        | `8080`                       | HTTP server port                                    |
| `RETRIEVE_MODE`      | ❌        | `local`                      | `local` (disk) or `github` (API)                    |
| `REPO_PATH`          | ❌        | `.`                          | Local repo path (local mode only)                   |
| `REPO_BRANCH`        | ❌        | repo default                 | Branch to scan (github mode only)                   |
| `REPO_EXTENSIONS`    | ❌        | `.go,.ts,.js,.py,...`        | Comma-separated file extensions to scan             |
| `WEBHOOK_ACTIONS`    | ❌        | `opened,edited`              | Issue actions that trigger the pipeline             |
| `OPENROUTER_REFERER` | ❌        | —                            | `HTTP-Referer` header sent to OpenRouter            |
| `OPENROUTER_TITLE`   | ❌        | `product-agent`              | `X-Title` header sent to OpenRouter                 |

---

## Test with curl

```bash
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issues" \
  -d '{
    "action": "opened",
    "issue": {
      "number": 1,
      "title": "Add rate limiting to the API",
      "body": "Need per-IP rate limiting with 429 + Retry-After.\n\nproduct-agent:comment"
    },
    "repository": {
      "name": "my-service",
      "owner": { "login": "my-org" }
    }
  }'
```

Expected response: `HTTP 202 Accepted`

The agent processes in the background and posts the comment within ~10 seconds.

---

## Run with Docker

```bash
docker build -t product-agent .

docker run --rm \
  --env-file .env \
  -p 8080:8080 \
  product-agent
```

---

## Project structure

```
product-agent/
├── main.go              # Entry point, Config struct, HTTP server
├── handler.go           # Webhook routing, pipeline orchestration
├── agent.go             # OpenAI-compatible LLM client
├── github.go            # GitHub API: parse webhook, post comment
├── retriever.go         # Local disk file scanner + repo tree + README reader
├── retriever_github.go  # GitHub API file retriever (Trees + Contents API)
├── indexer.go           # Auto-generates codebase index from function signatures
├── prompt.go            # LLM prompt construction
├── types.go             # Shared structs (Config, GitHubIssue, AgentOutput…)
└── utils.go             # Keyword extraction, Markdown formatting, helpers
```
