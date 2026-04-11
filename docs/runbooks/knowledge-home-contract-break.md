---
title: "Knowledge Home Contract Break"
date: 2026-03-18
tags:
  - runbook
  - knowledge-home
  - proto
  - ci
---

# Knowledge Home Contract Break

Runbook for resolving proto contract breakage detected by the CI workflow.

Related: [[000422]], [[000429]], [[knowledge-home-phase0-canonical-contract]]

## Symptoms

- CI failure in the `proto-contract` workflow.
- Error message indicates a breaking change in Knowledge Home proto definitions.
- Pull request is blocked from merging.

## Background

Knowledge Home uses protocol buffers to define the contract between alt-backend (server) and alt-frontend-sv (client). The CI pipeline runs `buf breaking` to detect backward-incompatible changes.

Breaking changes include:
- Removing a field.
- Renaming a field.
- Changing a field number.
- Changing a field type.
- Removing an enum value.
- Changing an RPC method signature.

Non-breaking changes include:
- Adding a new field (with a new field number).
- Adding a new enum value.
- Adding a new RPC method.
- Adding a new message type.

## Investigation

### 1. Identify the breaking change

```bash
# Check the CI output for the specific break
# Or run locally:
cd /home/koko/Documents/dev/Alt/proto
buf breaking --against '.git#branch=main'
```

The output will list specific files and fields that broke backward compatibility.

### 2. Review the git diff

```bash
cd /home/koko/Documents/dev/Alt/proto
git diff main -- .
```

Identify which field or message was modified.

### 3. Assess impact

| Change type | Impact | Action |
|-------------|--------|--------|
| Field removed | Clients using that field will break | Add back; deprecate instead |
| Field renamed | Wire format unchanged if number is same | Revert name; old name is wire-irrelevant but convention matters |
| Field number changed | Wire-incompatible | Revert to original number |
| Type changed | Deserialization will fail | Add new field with new number |
| Enum value removed | Clients using that value will break | Add back; deprecate instead |

## Resolution

### Add backward-compatible fields instead of modifying

Instead of renaming or removing a field:

```protobuf
message KnowledgeHomeItem {
  // DO NOT remove or rename existing fields.
  string old_field = 1 [deprecated = true];

  // Add new field with a new number.
  string new_field = 10;
}
```

### Deprecation workflow

1. Mark the old field as `deprecated = true` in the proto file.
2. Add the replacement field with a new field number.
3. Update server code to populate both old and new fields during the transition.
4. After all clients have migrated (minimum one release cycle), stop populating the deprecated field.
5. Never remove the field number from the proto definition.

### If removal is intentional

If the break is intentional (e.g., major version bump):

1. Confirm with the team that all consumers have been updated.
2. Add a `buf:lint:ignore` comment if appropriate.
3. Update the proto package version (e.g., `v1` -> `v2`).
4. Update all import paths in both server and client.

## Semantic Contract Checklist

Even when `buf breaking` passes, treat the change as unsafe if it breaks the Phase 0 canonical contract:

- `service_quality` is no longer propagated end-to-end
- `summary_state` values drift from `missing | pending | ready`
- `supersede_state` values drift from `summary_updated | tags_updated | both_updated`
- `why.code` introduces a new value without updating the canonical contract and frontend mappings
- stream business events and compatibility events are no longer distinguishable

When one of these occurs:

1. Update [[knowledge-home-phase0-canonical-contract]] first.
2. Update backend handler / frontend connect conversion in the same change.
3. Extend contract tests before merging.

## Prevention

- Always run `buf breaking` locally before pushing proto changes:
  ```bash
  cd /home/koko/Documents/dev/Alt/proto
  buf breaking --against '.git#branch=main'
  ```
- Use `reserved` to prevent accidental reuse of removed field numbers:
  ```protobuf
  message KnowledgeHomeItem {
    reserved 3, 7;
    reserved "removed_field_name";
  }
  ```
- Review proto changes with both backend and frontend engineers before merging.

## Verification

- CI `proto-contract` check passes.
- `buf breaking` returns no errors locally.
- Both alt-backend and alt-frontend-sv build successfully with the updated proto.
