# Hatchet Skill for Claude Code

A Claude Code plugin that provides expert assistance for building and debugging [Hatchet](https://hatchet.run) workflows.

> **Note:** This is a community plugin, not affiliated with or endorsed by Hatchet or Anthropic.

## What is Hatchet?

Hatchet is a platform for background tasks and durable execution built on PostgreSQL. You write tasks in Python, TypeScript, or Go and run them on workers in your infrastructure.

## Installation

Install this plugin using the Claude Code marketplace:

```
/plugin marketplace add igor-kupczynski/hatchet-skill
```

```
/plugin install hatchet@hatchet-skill
```

Restart `claude` and verify the skill is present in `/skills`.

## What This Skill Helps With

- Setting up Hatchet SDK (Python, TypeScript, Go)
- Creating DAG workflows with task dependencies
- Configuring workers and slots
- Setting up event triggers and cron schedules
- Implementing rate limiting and concurrency control
- Building durable tasks for long-running operations
- Debugging workflow runs and connection issues
- Both Hatchet Cloud and self-hosted deployments

## Usage

Once installed, Claude will automatically use this skill when you ask questions about Hatchet workflows. For example:

- "How do I create a Hatchet workflow with multiple steps?"
- "Help me set up rate limiting for my Hatchet task"
- "My Hatchet worker isn't connecting, how do I debug it?"
- "Show me how to use durable tasks for a long-running process"

## License

MIT
