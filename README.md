<p align="center">
  <img width="1024" height="340" alt="image" src="https://github.com/user-attachments/assets/32ed8985-841d-49c3-81f7-2aabc7c7c564" />
</p>

<p align="center">
  <strong>Persistent memory + knowledge graph for AI coding agents</strong><br>
  <em>Fork of <a href="https://github.com/Gentleman-Programming/engram">engram</a>. Agent-agnostic. Single binary. Zero CGO.</em>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#architecture">Architecture</a> &bull;
  <a href="#mcp-tools">MCP Tools</a> &bull;
  <a href="#knowledge-graph">Knowledge Graph</a> &bull;
  <a href="#profiles">Profiles</a> &bull;
  <a href="docs/AGENT-SETUP.md">Agent Setup</a> &bull;
  <a href="DOCS.md">Full Docs</a>
</p>

---

> **Mneme** `/ˈniːm.iː/` — *Greek mythology*: one of the three Muses of memory.

Your AI coding agent forgets everything when the session ends. Mneme gives it a brain — **with a knowledge graph**.

A **Go binary** with SQLite + FTS5 full-text search and a bi-temporal knowledge graph layer (entities, relations, communities). Exposed via CLI, HTTP API, MCP server, and an interactive TUI. Works with **any agent** that supports MCP — Claude Code, OpenCode, Gemini CLI, Codex, VS Code, Cursor, or anything else.

```
Agent (Claude Code / OpenCode / Gemini CLI / ...)
    |  MCP stdio
Mneme (single Go binary, modernc.org/sqlite — NO CGO)
    |
    +-- observations  (FTS5 full-text search)
    +-- entities      (knowledge graph nodes)
    +-- relations     (bi-temporal directed edges)
    +-- communities   (union-find connected components)
```

## Quick Start

### Install

```bash
# Build from source
git clone https://github.com/Edcko/Mneme.git
cd Mneme && go build -o mneme ./cmd/mneme
```

### Setup Your Agent

| Agent | One-liner |
|-------|-----------|
| Claude Code | `claude plugin marketplace add Edcko/Mneme && claude plugin install mneme` |
| OpenCode | `mneme setup opencode` |
| Gemini CLI | `mneme setup gemini-cli` |
| VS Code | `code --add-mcp '{"name":"mneme","command":"mneme","args":["mcp"]}'` |
| Cursor / Any MCP | See [docs/AGENT-SETUP.md](docs/AGENT-SETUP.md) |

No Node.js, no Python, no Docker, no CGO. **One binary, one SQLite file.**

## Architecture

```
                      MCP Tool Call
                           |
              +------------+------------+
              |                         |
         Memory Layer            Graph Layer
              |                         |
      mem_save / mem_update    IndexObservationEntities
              |                   (async goroutine)
              v                         |
    +-------------------+     RuleExtractor (zero deps)
    |  observations     |        |              |
    |  (SQLite + FTS5)  |   Gazetteer        Regex
    +-------------------+   (60+ tools)   (paths, decisions)
              |                  |              |
              +--------+---------+--------------+
                       |
              +--------+--------+
              |  Knowledge Graph |
              |  ┌─────────────┐ |
              |  │  entities    │ │  6 types: person, project,
              |  │  (FTS5 idx)  │ │  file, tool, concept, language
              |  ├─────────────┤ |
              |  │  relations   │ │  bi-temporal (t_valid + t_invalid)
              |  │  (directed)  │ │  auto-supersede on insert
              |  ├─────────────┤ |
              |  │ communities  │ │  union-find connected components
              |  │  (members)   │ │  rebuilt on demand
              |  └─────────────┘ |
              +------------------+
                       |
              +--------+--------+
              |  Query Engine   |
              |  BFS traversal  |  Recursive CTE, max depth 10, cycle-safe
              |  FTS5 search    |  Entities + observations
              |  Bi-temporal    |  "what was true when" queries
              +-----------------+
```

**Key design decisions:**

- **100% backward compatible** — all 15 original engram tools unchanged
- **Async extraction** — `IndexObservationEntities` runs in a goroutine after `mem_save`. Errors are swallowed to never block the primary flow
- **Bi-temporal relations** — `t_valid` (when it became true) + `t_invalid` (when superseded). Relations are never deleted, only invalidated
- **Auto-supersede** — inserting a new relation of the same type from the same source invalidates the old one (e.g., "project uses PostgreSQL" supersedes "project uses SQLite")
- **Zero external dependencies** — `modernc.org/sqlite` (pure Go), no CGO, no LLM calls for extraction

## MCP Tools

21 tools total: 15 memory + 6 graph.

### Memory Tools (from engram)

| Tool | Profile | Purpose |
|------|---------|---------|
| `mem_save` | agent | Save observation (title, type, What/Why/Where/Learned) |
| `mem_search` | agent | Full-text search across observations |
| `mem_update` | agent | Update observation by ID |
| `mem_delete` | admin | Soft or hard delete |
| `mem_suggest_topic_key` | agent | Stable key for evolving topics |
| `mem_session_summary` | agent | End-of-session structured save |
| `mem_context` | agent | Recent session context |
| `mem_timeline` | admin | Chronological drill-in around an observation |
| `mem_get_observation` | agent | Full content by ID |
| `mem_save_prompt` | agent | Save user prompt |
| `mem_capture_passive` | agent | Extract learnings from text output |
| `mem_stats` | admin | Memory statistics |
| `mem_session_start` | agent | Register session start |
| `mem_session_end` | agent | Mark session complete |
| `mem_merge_projects` | admin | Merge project name variants |

### Graph Tools (new in Mneme)

| Tool | Profile | Purpose |
|------|---------|---------|
| `mem_graph_search` | graph | BFS traversal from a seed entity (default depth 3, max 10) |
| `mem_entities` | graph | List/search entities by type, project, or FTS5 query |
| `mem_relations` | graph | Show relations for an entity (active or with history) |
| `mem_relation_history` | graph | Bi-temporal history of a specific relation |
| `mem_invalidate` | graph | Mark a relation as superseded (preserves history) |
| `mem_rebuild_communities` | graph | Recompute connected components via union-find |

## Knowledge Graph

### Entities

Nodes in the graph, automatically extracted from every `mem_save` call. Six entity types:

| Type | Examples | Detection |
|------|----------|-----------|
| `tool` | SQLite, Docker, React, Next.js, FTS5, MCP | Gazetteer (60+ entries) |
| `language` | Go, TypeScript, Python, Rust | Gazetteer (20+ entries) |
| `file` | `internal/store/graph.go`, `src/App.tsx` | Regex pattern |
| `project` | `github.com/Edcko/Mneme` | Regex pattern |
| `person` | — | Future: LLM extraction |
| `concept` | — | Future: LLM extraction |

Entity deduplication uses case-insensitive `(name, project)` uniqueness. An FTS5 virtual table enables fast text search across entity names and summaries.

### Relations

Directed, bi-temporal edges between entities. Automatically inferred from observation text:

| Relation Rule | Example |
|---------------|---------|
| `reemplaza_a` (replaces) | "Switched to **PostgreSQL** instead of **SQLite**" |
| `depende_de` (depends on) | "**Service** depends on **Redis**" |
| `arreglado_en` (fixed in) | "Fixed N+1 query in **store.go**" |
| `extiende` (extends) | "**Handler** extends **BaseController**" |

### Bi-temporal Tracking

Every relation tracks two timestamps:

```
t_valid:     when the relation became true
t_invalid:   when it was superseded (NULL = still active)
```

This enables "what was true when" queries. Example: a project that migrated from SQLite to PostgreSQL has both relations in history — the old one with `t_invalid` set, the new one active.

### Communities

Connected components computed via union-find with path compression. Only communities with 2+ members are stored. Rebuilt on demand via `mem_rebuild_communities`.

### Extraction Pipeline

```
mem_save(content="Switched to PostgreSQL instead of SQLite")
    |
    +-- [sync] Persist observation to SQLite + FTS5
    |
    +-- [async goroutine] IndexObservationEntities
            |
            Layer 1: Gazetteer lookup
              -> finds "PostgreSQL" (tool, 0.95), "SQLite" (tool, 0.95)
            |
            Layer 2: Regex patterns
              -> no file paths or decisions in this text
            |
            Relation rules:
              -> "Switched to X instead of Y" -> PostgreSQL -[reemplaza_a]-> SQLite
            |
            Upsert entities + insert relation
```

The `Extractor` interface is pluggable — swap `RuleExtractor` for an LLM-based one without changing callers.

## Profiles

Tool profiles control which MCP tools are registered per connection:

```bash
mneme mcp --tools=agent        # 11 memory tools (AI coding sessions)
mneme mcp --tools=admin        # 5 curation/stats tools (TUI, dashboards)
mneme mcp --tools=graph        # 6 knowledge graph tools
mneme mcp --tools=agent,graph  # combine profiles
mneme mcp                      # all tools (default)
```

| Profile | Tools | Use case |
|---------|-------|----------|
| `agent` | 11 | AI agent coding sessions |
| `admin` | 5 | TUI, dashboards, manual curation |
| `graph` | 6 | Knowledge graph exploration |

## CLI Reference

| Command | Description |
|---------|-------------|
| `mneme setup [agent]` | Install agent integration |
| `mneme serve [port]` | Start HTTP API (default: 7437) |
| `mneme mcp` | Start MCP server (stdio) |
| `mneme tui` | Launch terminal UI |
| `mneme search <query>` | Search memories |
| `mneme save <title> <msg>` | Save a memory |
| `mneme timeline <obs_id>` | Chronological context |
| `mneme context [project]` | Recent session context |
| `mneme stats` | Memory statistics |
| `mneme version` | Show version |

## Documentation

| Doc | Description |
|-----|-------------|
| [Agent Setup](docs/AGENT-SETUP.md) | Per-agent configuration + Memory Protocol |
| [Architecture](docs/ARCHITECTURE.md) | Internal architecture + project structure |
| [Contributing](CONTRIBUTING.md) | Contribution workflow + standards |
| [Full Docs](DOCS.md) | Complete technical reference |

## License

MIT

---

**Forked from [engram](https://github.com/Gentleman-Programming/engram)** — persistent memory for AI agents. Mneme adds a bi-temporal knowledge graph layer with zero external dependencies.
