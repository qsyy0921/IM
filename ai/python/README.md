# NexusIM Python AI Worker Foundation

This directory is the Python side of ADR-036.

Python workers are candidate generators only. Go services keep authority over
tenant identity, policy, approval, audit, persistence, outbox and repair.

## Allowed Work

- LLM provider adapter candidates.
- Embedding and rerank candidates.
- Memory extraction and profile aggregation candidates.
- Planner / critic / verifier prototypes.
- Offline eval and benchmark processing.

## Forbidden Work

- Direct reads or writes of NexusIM business PostgreSQL tables.
- Final approval, execution, memory, summary, answer or proposal state.
- High-risk business actions.
- Persisting raw prompts, raw provider bodies, secrets, tokens or sensitive
  payloads.

## First-Stage Layout

```text
ai/python/
  environment.yml
  pyproject.toml
  nexusim_ai_common/
    contracts.py
    safety.py
  contracts/
    worker-candidate.schema.json
  scripts/
    validate_contracts.py
  tests/
```

## Local Toolchain

All Python tools for this repo should run inside the `IM` conda environment:

```powershell
conda env create -f ai/python/environment.yml
conda activate IM
python -m pytest ai/python/tests -q
python ai/python/scripts/validate_contracts.py
python -m ruff check ai/python
python -m mypy ai/python/nexusim_ai_common ai/python/scripts
```

If the environment already exists, update it instead:

```powershell
conda env update -n IM -f ai/python/environment.yml
```

The environment installs `nexusim-ai-workers` in editable mode and keeps
Python tooling out of the system Python installation.

## Boundary Guard

Run this repo-level guard when changing Python worker foundations:

```powershell
.\tools\check-python-ai-worker-boundary.ps1
```

The guard is intentionally small. It protects the first-stage boundary:
candidate-only Python, Go-owned control plane, no direct IM PostgreSQL table
access from Python workers, and a reproducible `IM` conda environment.
