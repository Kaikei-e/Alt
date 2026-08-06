#!/usr/bin/env bash
# Remove every container, network and volume belonging to a compose project.
#
#   bash e2e/_lib/teardown-project.sh <project-name>
#
# The suites' own EXIT trap already tears the stack down (`docker compose down
# -v` against the rendered slice), so this is the belt to that braces: CI runs
# the suites with KEEP_STACK=1 so a failure handler can still read the logs,
# which means nothing has cleaned up by the time the job ends.
#
# It goes through the compose project *label* rather than a compose file
# because by this point there is no compose file left — the slice lived under
# `mktemp -d` and the trap deleted it. Labels survive; paths do not.
#
# Every step is `|| true`. A teardown that fails the job it is cleaning up
# after replaces a real test failure with a cleanup failure in the CI summary,
# which is the one thing this must never do.
set -uo pipefail

PROJECT="${1:?usage: teardown-project.sh <project-name>}"
FILTER="label=com.docker.compose.project=$PROJECT"

containers="$(docker ps -aq --filter "$FILTER" 2>/dev/null || true)"
if [[ -n "$containers" ]]; then
  # shellcheck disable=SC2086  # word splitting is what turns the list into args
  docker rm -f $containers >/dev/null 2>&1 || true
fi

volumes="$(docker volume ls -q --filter "$FILTER" 2>/dev/null || true)"
if [[ -n "$volumes" ]]; then
  # shellcheck disable=SC2086
  docker volume rm -f $volumes >/dev/null 2>&1 || true
fi

# The network is named after the project (e2e/_lib/render-slice.sh rewrites it),
# so it is not necessarily labelled with it — remove it by name as well.
networks="$(docker network ls -q --filter "$FILTER" 2>/dev/null || true)"
if [[ -n "$networks" ]]; then
  # shellcheck disable=SC2086
  docker network rm $networks >/dev/null 2>&1 || true
fi
docker network rm "$PROJECT" >/dev/null 2>&1 || true

exit 0
