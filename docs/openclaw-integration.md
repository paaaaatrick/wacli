# OpenClaw Integration Notes

This branch tracks a concrete downstream use case for `wacli`:

- one WhatsApp account is used as the live assistant front door
- a separate personal WhatsApp account is mirrored as read-only evidence
- downstream retrieval wants structured local exports, not human CLI text

## What OpenClaw needs most

1. Stable machine-readable lifecycle events for auth and sync.
2. Reliable long-running follow mode without stale lock confusion.
3. Better LID resolution and sender/chat naming.
4. Good extraction for WhatsApp Business message types.
5. A stable export surface for downstream ingestion tools.
6. Clear reply/context metadata so downstream systems can preserve thread structure.

## Why this matters

The alternative is to scrape session logs or human-readable CLI output. That is brittle and mixes transport behavior with retrieval logic.

`wacli` is much more promising as a local linked-device ingestion sidecar:

- local SQLite store
- follow mode
- best-effort history sync
- machine-readable command output

## Near-term downstream plan

- probe the local store for health and recent activity
- export messages into a normalized ingestion adapter
- shadow the adapter against the existing WhatsApp evidence pipeline
- cut over only after parity is proven

## Candidate upstream improvements to watch

- PR #85: machine-readable lifecycle events
- PR #79: WhatsApp Business message text extraction
- PR #92: send while `sync --follow` is running

## Open question

If `wacli` becomes primarily maintained through forks, it may be worth keeping a small, disciplined downstream patch stack instead of waiting for upstream merges.
