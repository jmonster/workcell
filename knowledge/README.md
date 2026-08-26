# Knowledge

Knowledge is the query contract I use to give agents citable domain evidence before they make a technical decision.

I have applied it successfully to Apple development, biology, chemistry, physics, mathematics, protocols, media, web platforms, design, survival, and pool chemistry. Each application owns its ingestion, schema, and ranking. Knowledge normalizes what comes back so the agent does not need a different integration for every subject.

## Query contract

A query begins with a limited result set. The agent reads the titles, source references, and excerpts, then explicitly expands a promising result when it needs more context. A usable hit has this shape:

```json
{
  "kb": "apple",
  "type": "section",
  "docs_url": "https://developer.apple.com/documentation/...",
  "anchor_url": "https://developer.apple.com/documentation/...#...",
  "section_id": "...",
  "title": "...",
  "text": "...",
  "score": -7.2,
  "extras": {}
}
```

`docs_url` identifies the source and `text` carries the supporting material. If either is missing, the agent treats the result as a gap to refine rather than an answer to repeat.

## How it serves the work

The agent consults the relevant knowledge base when a task depends on exact behavior or specialized facts. The first query stays small enough to preserve working context; expansion is a separate decision. Source-specific retrieval remains behind the common response shape instead of forcing every subject into one schema or ranking model.

This is designed to let small, cheap, local agents retrieve specialist material at the point of work instead of carrying every domain in their model weights.

## Limits

Retrieval does not replace judgment. A high-ranked excerpt can still be incomplete, and no result does not mean no answer exists. Scores are source-specific and are not comparable across knowledge bases. Provenance makes the evidence inspectable; it does not guarantee that the underlying source is current or correct.
