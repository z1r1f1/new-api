# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

This directory contains guidelines for backend development. Fill in each file with your project's specific conventions.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Module organization and file layout | Filled from codebase examples |
| [Database Guidelines](./database-guidelines.md) | ORM patterns, queries, migrations | Filled from codebase examples |
| [Error Handling](./error-handling.md) | Error types, handling strategies | Filled from codebase examples |
| [Quality Guidelines](./quality-guidelines.md) | Code standards, forbidden patterns | Filled from codebase examples |
| [Logging Guidelines](./logging-guidelines.md) | Structured logging, log levels | Filled from codebase examples |

---

## Pre-Development Checklist

Before writing backend code, read the files that match the change:

- Always read [Directory Structure](./directory-structure.md) to keep code in the correct layer.
- Read [Database Guidelines](./database-guidelines.md) for any change touching `model/`, migrations, GORM queries, log persistence, or raw SQL.
- Read [Error Handling](./error-handling.md) for controller, middleware, relay, service, or provider-adapter error paths.
- Read [Logging Guidelines](./logging-guidelines.md) before adding process logs, request logs, persistent usage/error logs, or admin audit data.
- Read [Quality Guidelines](./quality-guidelines.md) before implementation and again before verification.

Also read shared thinking guides under `.trellis/spec/guides/`, especially the cross-layer guide for changes that span router/controller/service/model/frontend boundaries.

---

## How to Fill These Guidelines

For each guideline file:

1. Document your project's **actual conventions** (not ideals)
2. Include **code examples** from your codebase
3. List **forbidden patterns** and why
4. Add **common mistakes** your team has made

The goal is to help AI assistants and new team members understand how YOUR project works.

---

**Language**: All documentation should be written in **English**.
