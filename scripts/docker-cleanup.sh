#!/bin/bash
# =============================================================================
# Docker Disk Space Cleanup Script
# Purpose: Reclaim Docker disk on a host that also runs the production compose
#          stack, without ever removing something a service needs to restart.
#
# What this never removes:
#   - An image that any container refers to, whatever that container's state.
#     The protected set is computed before the first deletion, so a service
#     that is merely between restarts keeps the image it needs to come back up.
#     This is the failure mode this script itself caused: a stopped container
#     was pruned first, which left its image unreferenced, and an unfiltered
#     `docker image prune -a -f` then deleted the only local copy, so
#     `restart: unless-stopped` had nothing to start.
#   - A container that belongs to a compose project, in any state. A stopped
#     compose service is a service waiting to restart; compose owns it.
#   - Named volumes. `docker volume prune` is deliberately not used anywhere
#     here: it deletes by "unreferenced", and an application volume becomes
#     unreferenced the moment its container is stopped or recreated. The one
#     narrow exception is orphaned buildx builder state volumes, matched by
#     name. Same argument as hosted_runner/gh-runner-docker-gc.sh.
#
# Nothing younger than the keep-age is removed on either path. A freshly pulled
# image with no container yet is a deploy in flight, not garbage.
#
# When the budget cannot be met, the script reports what would have to be
# deleted to meet it, and deletes none of it. That judgement is the operator's.
#
# Usage:
#   docker-cleanup.sh             # reclaim
#   docker-cleanup.sh --dry-run   # log every action, perform none
#   docker-cleanup.sh --report    # shortfall report only, reclaim nothing
# =============================================================================

set -euo pipefail

# Configuration
MAX_DOCKER_SIZE_GB=${MAX_DOCKER_SIZE_GB:-100}
DOCKER_ROOT_DIR=${DOCKER_ROOT_DIR:-/var/lib/docker}
LOG_FILE=${LOG_FILE:-/var/log/docker-cleanup.log}

# Age floor for the routine path.
KEEP_AGE_HOURS=${DOCKER_CLEANUP_KEEP_AGE_HOURS:-24}
# The over-budget path may reach further back, but never to zero. An unbounded
# sweep buys a few hundred MB and risks the image of whatever restarted last.
AGGRESSIVE_KEEP_AGE_HOURS=${DOCKER_CLEANUP_AGGRESSIVE_KEEP_AGE_HOURS:-6}

# The compose project whose declared images are protected even when no
# container for them exists at all.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE=${COMPOSE_FILE:-$SCRIPT_DIR/../compose/compose.yaml}
COMPOSE_PROJECT=${COMPOSE_PROJECT:-alt}

LOCKFILE=${DOCKER_CLEANUP_LOCKFILE:-/var/run/docker-cleanup.lock}
# The full ranked shortfall report is verbose and the timer runs every 30
# minutes, so throttle it. The one-line summary is always logged.
REPORT_INTERVAL_HOURS=${DOCKER_CLEANUP_REPORT_INTERVAL_HOURS:-6}
REPORT_STAMP=${DOCKER_CLEANUP_REPORT_STAMP:-/var/run/docker-cleanup.report-stamp}

DRY_RUN=false
REPORT_ONLY=false
COMPOSE_PROTECTION=unavailable

# Convert GB to bytes
MAX_SIZE_BYTES=$((MAX_DOCKER_SIZE_GB * 1024 * 1024 * 1024))

declare -A PROTECTED_IMAGE_IDS=()

# Logging function. Writes to stderr rather than stdout so that helpers can be
# called inside command substitution without their diagnostics contaminating
# the value. Under systemd both streams land in the journal.
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE" >&2
}

# Swallows command output but not the reason for a failure, and keeps the
# DRY-RUN line out of any redirection the caller might otherwise apply.
run() {
    if [ "$DRY_RUN" = "true" ]; then
        log "DRY-RUN: $*"
        return 0
    fi
    local out rc=0
    out=$("$@" 2>&1) || rc=$?
    [ "$rc" -eq 0 ] || log "command_failed rc=${rc} cmd=$* out=${out}"
    return "$rc"
}

fmt() {
    numfmt --to=iec-i --suffix=B "${1:-0}" 2>/dev/null || echo "${1:-0}"
}

# Docker prints sizes as SI strings ("17.64GB", "1.74kB"). numfmt wants an
# uppercase suffix and no trailing B.
to_bytes() {
    local s="${1:-0}"
    s="${s%B}"
    s="${s/k/K}"
    [ -n "$s" ] || { echo 0; return; }
    numfmt --from=si --round=nearest "$s" 2>/dev/null || echo 0
}

# Get current Docker disk usage in bytes
get_docker_size() {
    du -sb "$DOCKER_ROOT_DIR" 2>/dev/null | cut -f1 || echo "0"
}

# -----------------------------------------------------------------------------
# Protection
# -----------------------------------------------------------------------------

# Builds the set of image IDs that must survive this run. Called once, before
# anything is deleted, so removing a container later in the run cannot make its
# image eligible in the same run.
#
# Source 1 (load-bearing): every container on the host, whatever its state.
#   This is the only source that holds up when a deploy pins images to a tag or
#   digest the compose file does not name.
# Source 2 (best effort): the images the compose project declares, which also
#   covers a service whose container has been removed entirely. This renders
#   with whatever environment the caller has, so it can resolve to tags that do
#   not exist locally; it can only ever add protection, never remove it.
collect_protected_images() {
    PROTECTED_IMAGE_IDS=()
    local -A refs=()
    local cid ref id compose_out

    while read -r cid; do
        [ -n "$cid" ] || continue
        while read -r ref; do
            [ -n "$ref" ] && [ "$ref" != "<no value>" ] && refs["$ref"]=1
        done < <(docker container inspect \
            --format '{{.Image}}{{"\n"}}{{.Config.Image}}' "$cid" 2>/dev/null || true)
    done < <(docker ps -a -q 2>/dev/null || true)

    if [ ! -f "$COMPOSE_FILE" ]; then
        COMPOSE_PROTECTION=unavailable
        log "compose_image_protection=unavailable reason=compose_file_missing path=${COMPOSE_FILE}"
    elif ! compose_out=$(docker compose -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" config --images 2>/dev/null); then
        COMPOSE_PROTECTION=unavailable
        log "compose_image_protection=unavailable reason=config_render_failed file=${COMPOSE_FILE} project=${COMPOSE_PROJECT}"
    else
        COMPOSE_PROTECTION=available
        while read -r ref; do
            [ -n "$ref" ] && refs["$ref"]=1
        done <<<"$compose_out"
        log "compose_image_protection=available file=${COMPOSE_FILE} project=${COMPOSE_PROJECT}"
    fi

    for ref in "${!refs[@]}"; do
        id=$(docker image inspect --format '{{.Id}}' "$ref" 2>/dev/null || true)
        [ -n "$id" ] && PROTECTED_IMAGE_IDS["$id"]=1
    done

    log "protected_images=${#PROTECTED_IMAGE_IDS[@]} image_refs_seen=${#refs[@]} compose_protection=${COMPOSE_PROTECTION}"
}

# -----------------------------------------------------------------------------
# Sweeps
# -----------------------------------------------------------------------------

# Removes stopped containers that no compose project owns, and only once they
# have been stopped for longer than the keep-age. Compose-managed containers
# are skipped in every state: pruning one is what unreferenced the image in the
# incident, and compose recreates them anyway.
sweep_containers() {
    local cutoff="$1"
    local removed=0 skipped=0
    local cid meta project name created finished ts

    while read -r cid; do
        [ -n "$cid" ] || continue
        meta=$(docker container inspect \
            --format '{{index .Config.Labels "com.docker.compose.project"}}|{{.Name}}|{{.Created}}|{{.State.FinishedAt}}' \
            "$cid" 2>/dev/null || true)
        [ -n "$meta" ] || continue
        IFS='|' read -r project name created finished <<<"$meta"

        if [ -n "$project" ] && [ "$project" != "<no value>" ]; then
            skipped=$((skipped + 1))
            log "container_kept=${name#/} reason=compose_project=${project}"
            continue
        fi

        ts=$(newest_epoch "$created" "$finished")
        if [ "$ts" -ge "$cutoff" ]; then
            skipped=$((skipped + 1))
            log "container_kept=${name#/} reason=younger_than_keep_age"
            continue
        fi

        log "container_remove=${name#/}"
        if run docker rm "$cid"; then
            removed=$((removed + 1))
        else
            log "container_remove_failed=${name#/} (left in place)"
        fi
    done < <(docker ps -a -q --filter status=exited --filter status=created --filter status=dead 2>/dev/null || true)

    log "containers_removed=${removed} containers_kept=${skipped}"
}

newest_epoch() {
    local a b ea eb
    a="${1:-}"
    b="${2:-}"
    ea=$(date -d "$a" +%s 2>/dev/null || echo 0)
    eb=$(date -d "$b" +%s 2>/dev/null || echo 0)
    if [ "$eb" -gt "$ea" ]; then echo "$eb"; else echo "$ea"; fi
}

# Replaces `docker image prune -a -f`. Removes only images that no container
# refers to, that the compose project does not declare, and that are older than
# the keep-age. `docker rmi` is called without -f on purpose: if anything still
# holds a reference the daemon refuses, which is the outcome we want.
sweep_images() {
    local cutoff="$1"
    local removed=0 kept=0
    local container_count img_id created_raw ref created_epoch target

    container_count=$(docker ps -a -q 2>/dev/null | wc -l)
    if [ "$container_count" -gt 0 ] && [ "${#PROTECTED_IMAGE_IDS[@]}" -eq 0 ]; then
        log "image_sweep=aborted reason=protected_set_empty containers=${container_count}"
        log "Refusing to remove images without a usable protected set."
        return 0
    fi

    while IFS='|' read -r img_id created_raw ref; do
        [ -n "$img_id" ] || continue
        if [ -n "${PROTECTED_IMAGE_IDS[$img_id]:-}" ]; then
            kept=$((kept + 1))
            continue
        fi
        created_epoch=$(date -d "$(echo "$created_raw" | cut -d' ' -f1-3)" +%s 2>/dev/null || echo 0)
        if [ "$created_epoch" -ge "$cutoff" ]; then
            kept=$((kept + 1))
            log "image_kept=${ref} reason=younger_than_keep_age"
            continue
        fi
        if [ "$ref" = "<none>:<none>" ]; then target="$img_id"; else target="$ref"; fi
        log "image_remove=${target}"
        if run docker rmi "$target"; then
            removed=$((removed + 1))
        else
            log "image_remove_failed=${target} (still referenced; left in place)"
        fi
    done < <(docker images --no-trunc --format '{{.ID}}|{{.CreatedAt}}|{{.Repository}}:{{.Tag}}' 2>/dev/null || true)

    log "images_removed=${removed} images_kept=${kept}"
}

# The only volume removal this script performs. buildx leaves a
# buildx_buildkit_builder-*_state volume behind when its builder container is
# killed before the teardown step runs. Matched by name and only when no
# container references it. Every other volume is reported, never removed.
reap_orphaned_builder_volumes() {
    local vol removed=0
    while read -r vol; do
        [ -n "$vol" ] || continue
        if [ -n "$(docker ps -a -q --filter "volume=${vol}" 2>/dev/null)" ]; then
            continue
        fi
        log "builder_volume_remove=${vol}"
        if run docker volume rm "$vol"; then
            removed=$((removed + 1))
        fi
    done < <(docker volume ls -q --filter 'name=buildx_buildkit_builder-' 2>/dev/null || true)
    log "builder_volumes_removed=${removed}"
}

# Build cache is always reproducible, but an unfiltered sweep also throws away
# the cache an in-flight build is about to reuse, so it stays age-filtered.
sweep_build_cache() {
    local age_hours="$1"
    log "Cleaning build cache older than ${age_hours}h..."
    run docker builder prune -a -f --filter "until=${age_hours}h" || true
}

sweep_networks() {
    log "Removing unused networks..."
    run docker network prune -f || true
}

# -----------------------------------------------------------------------------
# Reporting
# -----------------------------------------------------------------------------

# Emits "<bytes>|<human>|<label>" lines for everything that could still be
# freed, grouped by what it costs to free it.
unreferenced_images() {
    docker system df -v --format \
        '{{range .Images}}{{.Containers}}|{{.UniqueSize}}|{{.Repository}}:{{.Tag}}
{{end}}' 2>/dev/null |
        while IFS='|' read -r containers size ref; do
            [ -n "$size" ] || continue
            [ "$containers" = "0" ] || continue
            echo "$(to_bytes "$size")|${size}|image ${ref}"
        done
}

volumes_by_size() {
    local want_links="$1" # "0" for unattached, "+" for attached
    docker system df -v --format \
        '{{range .Volumes}}{{.Links}}|{{.Size}}|{{.Name}}
{{end}}' 2>/dev/null |
        while IFS='|' read -r links size name; do
            [ -n "$size" ] || continue
            if [ "$want_links" = "0" ]; then
                [ "$links" = "0" ] || continue
            else
                [ "$links" != "0" ] || continue
            fi
            echo "$(to_bytes "$size")|${size}|volume ${name} (links=${links})"
        done
}

container_layers_by_size() {
    docker ps -as --format '{{.Names}}|{{.Size}}' 2>/dev/null |
        while IFS='|' read -r name size; do
            [ -n "$size" ] || continue
            size="${size%% *}"
            echo "$(to_bytes "$size")|${size}|container ${name} writable layer"
        done
}

build_cache_total() {
    docker system df -v --format '{{range .BuildCache}}{{.Size}}
{{end}}' 2>/dev/null |
        while read -r size; do
            [ -n "$size" ] || continue
            echo "$(to_bytes "$size")|${size}|build cache entry"
        done
}

# Everything docker itself claims to be storing, which is the ceiling on what
# any prune can reach. The gap against du is the part no prune can touch.
docker_accounted_total() {
    local total=0 size
    while IFS='|' read -r _ size _; do
        [ -n "$size" ] || continue
        total=$((total + $(to_bytes "$size")))
    done < <(docker system df --format '{{.Type}}|{{.Size}}|{{.Reclaimable}}' 2>/dev/null || true)
    echo "$total"
}

# Prints the largest candidates first with a running total, so the operator can
# read straight off "these three get me back under budget".
print_ranked() {
    local heading="$1" shortfall="$2"
    local total=0 shown=0 bytes human label covered=false
    local sorted
    sorted=$(sort -t'|' -k1,1nr)
    if [ -z "$sorted" ]; then
        log "  ${heading}: none"
        RANKED_TOTAL=0
        return 0
    fi
    log "  ${heading}:"
    while IFS='|' read -r bytes human label; do
        [ -n "$bytes" ] || continue
        [ "$bytes" -gt 0 ] || continue
        total=$((total + bytes))
        shown=$((shown + 1))
        if [ "$shown" -le 12 ]; then
            if [ "$covered" = false ] && [ "$total" -ge "$shortfall" ] && [ "$shortfall" -gt 0 ]; then
                log "    $(printf '%10s' "$human")  running=$(fmt "$total")  ${label}   <== budget met here"
                covered=true
            else
                log "    $(printf '%10s' "$human")  running=$(fmt "$total")  ${label}"
            fi
        fi
    done <<<"$sorted"
    if [ "$shown" -gt 12 ]; then
        log "    ... and $((shown - 12)) more, group total $(fmt "$total")"
    fi
    RANKED_TOTAL="$total"
}

# Replaces "Manual intervention may be required" with the list of things whose
# deletion would actually close the gap, and what each one costs.
report_shortfall() {
    local current="$1"
    local shortfall=$((current - MAX_SIZE_BYTES))
    local safe=0 unattached=0 attached=0 layers=0 accounted=0 unreclaimable=0

    log "----------------------------------------------------------------"
    log "OVER BUDGET by $(fmt "$shortfall") (${DOCKER_ROOT_DIR}=$(fmt "$current"), limit=${MAX_DOCKER_SIZE_GB}GB)."
    log "This script has removed everything it can remove safely. What is left"
    log "is in use or is application data, so closing the gap is an operator"
    log "decision. To reach the target, delete from the lists below (largest"
    log "first) or raise MAX_DOCKER_SIZE_GB."

    log "[1] No data loss - images no container refers to, plus build cache:"
    print_ranked "unreferenced images" "$shortfall" < <(unreferenced_images)
    safe=$((safe + RANKED_TOTAL))
    print_ranked "build cache" "$shortfall" < <(build_cache_total)
    safe=$((safe + RANKED_TOTAL))

    log "[2] Verify first - volumes no container is attached to. These belong"
    log "    to services that are currently down, not necessarily to nothing:"
    print_ranked "unattached volumes" "$shortfall" < <(volumes_by_size 0)
    unattached="$RANKED_TOTAL"

    log "[3] Application data - deleting these loses state:"
    print_ranked "attached volumes" "$shortfall" < <(volumes_by_size +)
    attached="$RANKED_TOTAL"

    log "[4] Recreate the container to reset - writable layers:"
    print_ranked "container writable layers" "$shortfall" < <(container_layers_by_size)
    layers="$RANKED_TOTAL"

    accounted=$(docker_accounted_total)
    unreclaimable=$((current - accounted))
    log "[5] Outside docker's own accounting: $(fmt "$unreclaimable")."
    log "    du reports $(fmt "$current") for ${DOCKER_ROOT_DIR}; docker accounts"
    log "    for $(fmt "$accounted") across images, containers, volumes and build"
    log "    cache. The remainder is container logs, overlay2 metadata and daemon"
    log "    state, which no prune command reaches. If it is large, the budget"
    log "    cannot be met by pruning at all and the limit is the wrong lever."

    log "SUMMARY: need $(fmt "$shortfall")."
    log "    [1] costs nothing:        $(fmt "$safe")"
    log "    [2] down services' data:  $(fmt "$unattached")"
    log "    [3] live application data: $(fmt "$attached")"
    log "    [4] writable layers:      $(fmt "$layers")"
    if [ "$safe" -ge "$shortfall" ]; then
        log "    [1] alone closes the gap."
    else
        log "    [1] is not enough. Closing the gap means deleting from [2]/[3]/[4]"
        log "    - all of which is data - or raising MAX_DOCKER_SIZE_GB above"
        log "    $(((current / 1024 / 1024 / 1024) + 1))GB."
    fi
    log "----------------------------------------------------------------"
}

should_print_full_report() {
    local now stamp
    now=$(date +%s)
    if [ ! -f "$REPORT_STAMP" ]; then
        return 0
    fi
    stamp=$(cat "$REPORT_STAMP" 2>/dev/null || echo 0)
    [ -n "$stamp" ] || stamp=0
    [ $((now - stamp)) -ge $((REPORT_INTERVAL_HOURS * 3600)) ]
}

# -----------------------------------------------------------------------------

reclaim() {
    local age_hours="$1" cutoff
    cutoff=$(( $(date +%s) - age_hours * 3600 ))
    log "Reclaiming with keep-age ${age_hours}h..."
    collect_protected_images
    sweep_containers "$cutoff"
    sweep_images "$cutoff"
    sweep_build_cache "$age_hours"
    reap_orphaned_builder_volumes
    sweep_networks
    log "Reclamation pass completed"
}

usage() {
    echo "usage: $(basename "$0") [--dry-run|--report]" >&2
}

main() {
    case "${1:-}" in
        --dry-run) DRY_RUN=true ;;
        --report) REPORT_ONLY=true ;;
        "") ;;
        *)
            usage
            exit 1
            ;;
    esac

    if ! docker info >/dev/null 2>&1; then
        log "ERROR: docker daemon unreachable; nothing to do"
        exit 1
    fi

    if ! [ -w "$(dirname "$LOCKFILE")" ]; then
        LOCKFILE="${TMPDIR:-/tmp}/docker-cleanup.lock"
    fi
    exec 200>"$LOCKFILE"
    if ! flock -n 200; then
        log "another run holds the lock; exiting"
        exit 0
    fi

    local current_size
    current_size=$(get_docker_size)

    log "Current Docker disk usage: $(fmt "$current_size")"
    log "Maximum allowed: ${MAX_DOCKER_SIZE_GB}GB ($(fmt "$MAX_SIZE_BYTES"))"

    if [ "$REPORT_ONLY" = "true" ]; then
        if [ "$current_size" -gt "$MAX_SIZE_BYTES" ]; then
            report_shortfall "$current_size"
        else
            log "Disk usage is within limits; nothing to report."
        fi
        return 0
    fi

    if [ "$current_size" -gt "$MAX_SIZE_BYTES" ]; then
        log "WARNING: Docker disk usage exceeds ${MAX_DOCKER_SIZE_GB}GB limit."
        reclaim "$AGGRESSIVE_KEEP_AGE_HOURS"
    else
        log "Disk usage is within limits. Performing regular maintenance cleanup..."
        reclaim "$KEEP_AGE_HOURS"
    fi

    sleep 2
    current_size=$(get_docker_size)
    log "Disk usage after cleanup: $(fmt "$current_size")"

    local status=0
    if [ "$current_size" -gt "$MAX_SIZE_BYTES" ]; then
        if should_print_full_report; then
            report_shortfall "$current_size"
            date +%s >"$REPORT_STAMP" 2>/dev/null || true
        else
            log "OVER BUDGET by $(fmt $((current_size - MAX_SIZE_BYTES)))."
            log "Full reclamation report throttled to every ${REPORT_INTERVAL_HOURS}h."
            log "Run '$(basename "$0") --report' for it now."
        fi
        status=1
    else
        log "SUCCESS: Disk usage is now within limits"
    fi

    log "Final Docker disk usage summary:"
    docker system df 2>/dev/null | tee -a "$LOG_FILE" >&2 || true
    return "$status"
}

# Run main function
main "$@"
