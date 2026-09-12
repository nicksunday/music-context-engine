# Deferred: recommendation caching

Status: future work, not part of reference-led-recommendations implementation.

First improve reference-led retrieval and measure stage latency, provider calls, and repeated lookup patterns. Preserve bounded calls and deadlines during that work. Revisit caching when repeated external lookups materially contribute to wait time; if inference dominates, API caching alone will not solve latency.

## Measurements from reference-led recommendations

The pre-change baseline captured on 2026-09-11 used the exact Symphony X song
prompt with `qwen3:latest` and a three-minute Ollama timeout. Ollama was configured
and the model was available, but the request did not return within the five-minute
bounded observation window. The reconstructed album variant was not run after that
timeout. This is one live timeout observation, not a stable latency distribution;
no performance improvement is claimed from it.

The deterministic starter corpus passed 4/4 eligible assertions and the new
reference-led corpus passed 4/4. Browser coverage passed 8/8. Current bounded work
limits are four similarity seeds with up to fifteen neighbors each, a 48-record
discovery reconciliation pool, and two normal model calls. These measurements make
request/model timeout investigation the first priority; they do not yet establish
that Last.fm, MusicBrainz, or destination resolution dominates latency.

Next measurement priority is at least 20 comparable cold-provider and warm-provider
runs per mode, recording stage timings and actual calls. Only then should cache
targets be prioritized between Last.fm similarity, MusicBrainz catalog/release-group
lookups, destination resolution, and local derived context.

Potential targets, in order to validate from traces:

- Last.fm direct similarity responses keyed by provider, canonical reference identity, and query parameters.
- MusicBrainz catalog, release-group metadata, and verified identity lookups keyed by stable entity IDs and query shape.
- Streaming destination resolution keyed by exact verified identity and provider/storefront.
- Local derived context only if database profiling shows a meaningful cost.

Prefer reusable source facts over caching final personalized batches. Reapply current exclusions, examples, fit judgments, and constraints on every request. Do not reuse outdated personal decisions merely because the prompt text matches.

A future proposal should define bounded storage, TTLs, source timestamps, schema/provider version keys, invalidation, short-lived negative caching, in-flight request coalescing, and stale-on-error behavior. Never persist credentials in keys or payloads; never cache transient authentication failures as “no similar artists.”

Validate cold versus warm latency and hit rates alongside unchanged eligibility, fit, and variety. Include corrected feedback, changed avoid constraints, provider outages, expired entries, and identical concurrent requests. Choose concrete TTLs and latency targets from measured behavior rather than promising arbitrary speedups now.
