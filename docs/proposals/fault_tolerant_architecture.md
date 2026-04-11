# Fault-Tolerant Architecture Proposal: Alt Recap System

**Date**: 2025-12-11
**Based on Evaluation**: Job `2493dfa7` (0.0 Coverage)

## 1. Executive Summary
This document proposes a fault-tolerant architecture for the Alt Recap System. The analysis of Job `2493dfa7` identified the lack of granular checkpointing and error isolation as primary root causes for the complete system failure. The proposed architecture shifts from a **Linear Synchronous Pipeline** to a **State-Machine-Driven Asynchronous Workflow** with robust validation gates.

## 2. Failure Analysis (Job 2493dfa7)
-   **Symptom**: Job completed with `NumClusters > 0` but `NumBullets = 0` across all genres.
-   **Diagnosis**: The failure likely occurred during the `Reduce` (Summarization/Persist) phase.
-   **Root Causes**:
    1.  **Process Fragility**: A crash or timeout in the summarization loop caused the entire genre/job to fail without saving partial results.
    2.  **Lack of Validation**: Empty outputs were accepted as "success" (soft failure) rather than triggering a retry.
    3.  **Observability Gap**: No critical alert was raised until the final report showed 0.0 coverage.

## 3. Core Architectural Principles
To achieve true fault tolerance, we will adhere to the following principles:
-   **Checkpointing (Math/Stat Consistency)**: Every expensive computation (Embedding, Clustering) must be persisted immediately. "Compute once, persist forever."
-   **Isolation (Bulkheads)**: Failure in one genre (e.g., "Tech" times out) must not prevent "Science" from completing.
-   **Idempotency**: Retrying a failed job must be safe and result in the same consistent state.
-   **Validation Gates**: Statistical anomaly detection (e.g., output length vs input length) to catch "silent failures."

## 4. Proposed Architecture
### 4.1. Orchestration: The "Recap State Machine"
Replace the current sequential function calls in `recap-worker` with a persistent State Machine.

**States:**
1.  `CREATED`: Job initialized.
2.  `FETCHED`: Articles retrieved and stored.
3.  `CLASSIFIED`: Genres assigned.
4.  `CLUSTERED`: Clusters generated and persisted to `recap_cluster_evidence`.
5.  `SUMMARIZED_PARTIAL`: Summaries are being generated. Individual cluster successes are recorded.
6.  `COMPLETED`: All steps done.
7.  `FAILED_RECOVERABLE`: A step failed but can be retried.

**Implementation**:
-   Table: `recap_job_steps` (job_id, step_name, status, payload, created_at).
-   If the worker crashes, it reads `recap_job_steps` on boot and resumes from the last `SUCCESS` state.

### 4.2. The "Summarization Circuit Breaker"
The interaction with `news-creator` (LLM) is High Risk/High Latency.
-   **Worker Queue**: Instead of a simple `process_batches()`, use a durable queue (e.g., SQL-backed queue in Postgres/Redis).
-   **Circuit Breaker**: Track error rates from `news-creator`.
    -   If error rate > 20% in 1 minute -> **OPEN** circuit (Job pauses, alerts admin).
    -   After 5 mins -> **HALF-OPEN** (Try 1 request).
    -   Success -> **CLOSED** (Resume).

### 4.3. Data Validation Gates (Statistical Quality Control)
Before marking a step as "Success", run validity checks:
-   **Pre-Summarization Gate**:
    -   check: `cluster_size > 0`
    -   check: `token_count < context_limit` (Prevent hard errors)
-   **Post-Summarization Gate**:
    -   check: `num_bullets > 0`
    -   check: `entropy(summary_text) > threshold` (Detect repetitive loops)
    -   **Action**: If check fails, move to `FAILED_RECOVERABLE` and trigger retry with "Fallback Prompt" (simplified instruction).

## 5. Implementation Roadmap
### Phase 1: Observability & Validation (Immediate)
-   [ ] Add "Post-Summarization Gate" in `recap-worker`. Throw explicit error if `NumBullets == 0` so retries can happen.
-   [ ] Implement "Partial Save": Save summaries to DB *immediately* after each cluster is processed, not at the end of the loop.

### Phase 2: Resume Capability (Short-term)
-   [ ] Modify `recap-worker` to check for existing `recap_outputs` for the current JobID before generating. Skip already completed genres.

### Phase 3: Full State Machine (Long-term)
-   [ ] Refactor `recap-worker` pipeline into distinct `Task` units with individual persistence.
-   [ ] Introduce `recap_failures` table for Dead Letter Queue pattern.

## 6. Mathematical & Statistical Consistency
-   **Vector Integrity**: Ensure embeddings are normalized (L2) before storage to ensure consistent cosine similarity/distance metrics during checkpoints.
-   **Cluster Stability**: Use `DBCV` (Density-Based Clustering Validation) score as a quality gate. If score drops below threshold, trigger re-clustering with different parameters (Recursive Clustering).
