# Agent Dataset Reproducibility Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for open-dataset and synthetic fixture
reproducibility. This is not an accepted ADR, dataset contract, production eval
service, schema, migration or backend integration.

## Verdict

Conditionally passed for the dataset reproducibility rehearsal slice.

The fixture harness now proves, with low-sensitive refs only, that public-style
dataset exports and synthetic fixtures must carry dataset manifest, license,
snapshot hash, split manifest, import hash, adapter version and deterministic
report refs before they can support eval gate evidence.

This does not authorize use of real NexusIM IM data or production dataset
ingestion.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/dataset_reproducibility.py`
- `ai/python/tests/test_agent_eval_dataset_reproducibility.py`

Fixture:

- `ai/python/fixtures/agent_eval/dataset_reproducibility_rehearsal.json`

The helper verifies:

- dataset manifests are public or synthetic only and cannot include production
  data;
- dataset ground truth remains separated from product facts;
- manifests include license refs, snapshot hash refs, split manifest refs,
  import hash refs and adapter version refs;
- repeated eval / calibration reports must have the same report hash refs;
- backend or model-provider calls block dataset reproducibility evidence;
- raw payload retention blocks dataset reproducibility evidence;
- memory calibration public export must reconcile dataset source refs with
  per-dataset case counts;
- promotion gates block missing manifests, changed snapshots, import hash
  mismatch and non-deterministic reports.

## Review Closure

This closes the fixture-only portion of the dataset reproducibility hardening
gap:

- public-dataset-style eval evidence now carries manifest / snapshot / split /
  import / adapter version refs;
- memory calibration export now has explicit reproducibility rehearsal evidence;
- non-deterministic report evidence blocks promotion.

It does not close:

- production dataset ingestion;
- public dataset license legal review;
- ai-eval-service storage contract;
- production release gate integration.

## Next Evidence Target

The remaining next step should be main integration review of the ADR candidate
package and fixture evidence, or a focused contract/version review requested by
that review. Do not promote production contracts from this evidence alone.
