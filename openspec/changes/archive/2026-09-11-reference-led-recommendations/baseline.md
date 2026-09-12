# Recommendation baseline

Captured 2026-09-11 before changing recommendation behavior.

## Environment

- Database: disposable copy at `/tmp/music-context-baseline/music_vault.db`
- Database SHA-256: `76bc4600232422d0eb6e14af770e528da05e25d071b8377b85e093169ea62544`
- Executable: checked-in `/Users/nick/git/music-context-platform/bin/music-vault`
- Source build status: not available; installed Go is 1.25.4 while `go.mod` requires 1.25.5
- Ollama URL: `http://127.0.0.1:11434`
- Model: `qwen3:latest`
- Ollama status: configured and available; `qwen3:latest` reported by `/api/tags`
- Web endpoint: disposable listener on `127.0.0.1:18787`
- Ollama timeout setting: 3 minutes
- Requested limit: 6
- Captured context SHA-256: `2092f53851bf6f9e5dbbb161f03b9bcb319579ef6dddc18212aaf3d59f589d10`

No credentials or API keys are included in this record.

## Exact song prompt

> Give me a batch of songs that are technically proficient and metal, in the same vein as Symphony X

Expected adjacent bands supplied by the user for later manual evaluation, not hard-coded retrieval requirements:

- Rhapsody of Fire
- Ayreon
- Blind Guardian
- Dream Theater

### Current result

The request was sent to the current web workflow using song mode. It did not return within the bounded five-minute command window. No candidate output was captured. The disposable server was stopped after the timeout; production data was not changed.

## Album variant

This is a reconstructed variant, not wording supplied verbatim by the user:

> Give me a batch of albums that are technically proficient and metal, in the same vein as Symphony X and Children of Bodom

It was not run because the exact song request had already exceeded the bounded baseline window. It must be evaluated separately and labeled reconstructed in future reports.

## Reproduction notes

The disposable server was started with:

```sh
MUSIC_VAULT_DB_PATH=/tmp/music-context-baseline/music_vault.db \
  ./bin/music-vault web \
  --addr 127.0.0.1:18787 \
  --ollama-url http://127.0.0.1:11434 \
  --model qwen3:latest \
  --ollama-timeout 3m
```

The pre-change baseline therefore establishes model/configuration availability and a reproducible timeout outcome, but not a quality or yield score. Later before/after comparisons must distinguish this timeout from an empty successful batch.