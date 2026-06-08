#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

cpu_limits="${CPU_LIMITS:-1 2 4}"
memory_limits="${MEMORY_LIMITS:-256m 512m 1g}"
vu_steps="${VU_STEPS:-20 50 100}"
outbox_workers="${OUTBOX_WORKERS:-8}"
duration="${DURATION:-30s}"
stats_wait="${STATS_WAIT:-15s}"
conversation_count="${CONVERSATION_COUNT:-1000}"
min_success_rate="${MIN_SUCCESS_RATE:-0.99}"
max_p99_ms="${MAX_P99_MS:-1000}"
max_pending="${MAX_PENDING:-1000}"
docker_network="${DOCKER_NETWORK:-nexusim-local_default}"
pg_dsn="${PG_DSN:-postgres://nexusim:nexusim@postgres:5432/nexusim?sslmode=disable}"
kafka_brokers="${KAFKA_BROKERS:-kafka:29092}"
kafka_topic="${KAFKA_TOPIC:-conversation.timeline.events}"
batch_size="${BATCH_SIZE:-500}"
poll_interval="${POLL_INTERVAL:-200ms}"
failure_backoff="${FAILURE_BACKOFF:-1s}"
result_root="${RESULT_ROOT:-loadtest/results/docker-resource-matrix-$(date +%Y%m%d-%H%M%S)}"
message_image="${MESSAGE_IMAGE:-nexusim/message-service:local}"
loadtest_image="${LOADTEST_IMAGE:-nexusim/sendmessage-loadtest:local}"
skip_image_build="${SKIP_IMAGE_BUILD:-0}"

mkdir -p bin/linux logs "$result_root"

commit_short="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
commit_full="$(git rev-parse HEAD 2>/dev/null || echo unknown)"
git_status="$(git status --short 2>/dev/null || true)"
git_status_env="$(printf '%s' "$git_status" | tr '\n' '|' | sed 's/|$//')"
git_dirty=false
if [[ -n "$git_status" ]]; then
  git_dirty=true
fi

if [[ "$skip_image_build" != "1" ]]; then
  if [[ ! -x bin/linux/message-service || ! -x bin/linux/sendmessage-loadtest ]]; then
    if ! command -v go >/dev/null 2>&1; then
      echo "go is not available and bin/linux binaries are missing. Copy prebuilt linux binaries or install Go." >&2
      exit 1
    fi
    CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-arm64}" go build -o bin/linux/message-service ./services/message-service/cmd/message-service
    CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-arm64}" go build -o bin/linux/sendmessage-loadtest ./loadtest/sendmessage
  fi
  docker build -f deploy/docker/message-service.runtime.Dockerfile -t "$message_image" .
  docker build -f deploy/docker/sendmessage-loadtest.runtime.Dockerfile -t "$loadtest_image" .
fi

result_abs="$(cd "$result_root" && pwd)"
matrix_path="$result_root/docker-resource-matrix-summary.jsonl"
: > "$matrix_path"

json_bool() {
  if [[ "$1" == "true" ]]; then
    printf true
  else
    printf false
  fi
}

json_string() {
  python3 - "$1" <<'PY'
import json
import sys
print(json.dumps(sys.argv[1]))
PY
}

summary_field() {
  python3 - "$1" "$2" <<'PY'
import json
import sys
with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)
value = data
for part in sys.argv[2].split("."):
    value = value.get(part)
print("" if value is None else value)
PY
}

for cpu in $cpu_limits; do
  for memory in $memory_limits; do
    max_passing_vu=0
    for vu in $vu_steps; do
      safe_cpu="$(printf '%s' "$cpu" | tr -cd '[:alnum:]')"
      safe_memory="$(printf '%s' "$memory" | tr -cd '[:alnum:]')"
      run_name="cpu-${safe_cpu}-mem-${safe_memory}-vu-${vu}-$(date +%Y%m%d-%H%M%S)"
      container_prefix="nexusim-${run_name}"
      grpc_name="${container_prefix}-grpc"
      relay_name="${container_prefix}-relay"
      container_result_dir="/results/${run_name}"
      host_summary_path="${result_root}/${run_name}/sendmessage-summary.json"
      gomaxprocs="$(python3 - "$cpu" <<'PY'
import math
import sys
print(max(1, math.ceil(float(sys.argv[1]))))
PY
)"

      echo "Starting Docker resource run cpu=$cpu memory=$memory vu=$vu workers=$outbox_workers"
      docker run -d --rm \
        --name "$grpc_name" \
        --network "$docker_network" \
        --cpus "$cpu" \
        --memory "$memory" \
        -e GOMAXPROCS="$gomaxprocs" \
        -e NEXUSIM_MESSAGE_SERVICE_MODE=grpc \
        -e NEXUSIM_PG_DSN="$pg_dsn" \
        -e NEXUSIM_GRPC_ADDR=0.0.0.0:10495 \
        -e NEXUSIM_DEBUG_ADDR=0.0.0.0:10497 \
        "$message_image" >/dev/null

      docker run -d --rm \
        --name "$relay_name" \
        --network "$docker_network" \
        --cpus "$cpu" \
        --memory "$memory" \
        -e GOMAXPROCS="$gomaxprocs" \
        -e NEXUSIM_MESSAGE_SERVICE_MODE=outbox-relay \
        -e NEXUSIM_PG_DSN="$pg_dsn" \
        -e NEXUSIM_KAFKA_BROKERS="$kafka_brokers" \
        -e NEXUSIM_KAFKA_TOPIC="$kafka_topic" \
        -e NEXUSIM_OUTBOX_WORKERS="$outbox_workers" \
        -e NEXUSIM_OUTBOX_BATCH_SIZE="$batch_size" \
        -e NEXUSIM_OUTBOX_POLL_INTERVAL="$poll_interval" \
        -e NEXUSIM_OUTBOX_FAILURE_BACKOFF="$failure_backoff" \
        -e NEXUSIM_DEBUG_ADDR=0.0.0.0:10500 \
        "$message_image" >/dev/null

      set +e
      sleep 2
      docker run --rm \
        --network "$docker_network" \
        -v "${result_abs}:/results" \
        -e NEXUSIM_COMMIT="$commit_short" \
        -e NEXUSIM_COMMIT_FULL="$commit_full" \
        -e NEXUSIM_GIT_DIRTY="$git_dirty" \
        -e NEXUSIM_GIT_STATUS_SHORT="$git_status_env" \
        "$loadtest_image" \
        --target="${grpc_name}:10495" \
        --vus="$vu" \
        --duration="$duration" \
        --stats-wait="$stats_wait" \
        --conversation-count="$conversation_count" \
        --pg-dsn="$pg_dsn" \
        --service-metrics-url="http://${grpc_name}:10497/debug/metrics" \
        --relay-metrics-url="http://${relay_name}:10500/debug/metrics" \
        --result-dir="$container_result_dir"
      run_status=$?
      docker rm -f "$grpc_name" "$relay_name" >/dev/null 2>&1
      set -e
      if [[ "$run_status" -ne 0 ]]; then
        echo "loadtest failed for cpu=$cpu memory=$memory vu=$vu" >&2
        exit "$run_status"
      fi

      success_rate="$(summary_field "$host_summary_path" success_rate)"
      p95_ms="$(summary_field "$host_summary_path" p95_ms)"
      p99_ms="$(summary_field "$host_summary_path" p99_ms)"
      request_count="$(summary_field "$host_summary_path" request_count)"
      outbox_pending_count="$(summary_field "$host_summary_path" outbox_pending_count)"
      seq_alloc_avg_ms="$(summary_field "$host_summary_path" conversation_seq_alloc_latency_ms)"
      kafka_publish_avg_ms="$(summary_field "$host_summary_path" kafka_publish_latency_ms)"
      passed="$(python3 - "$success_rate" "$p99_ms" "$outbox_pending_count" "$min_success_rate" "$max_p99_ms" "$max_pending" <<'PY'
import sys
success_rate, p99_ms, pending, min_success, max_p99, max_pending = map(float, sys.argv[1:])
print("true" if success_rate >= min_success and p99_ms <= max_p99 and pending <= max_pending else "false")
PY
)"
      if [[ "$passed" == "true" ]]; then
        max_passing_vu="$vu"
      fi

      printf '{"cpu_limit":%s,"memory_limit":%s,"outbox_workers":%s,"vus":%s,"passed":%s,"success_rate":%s,"p95_ms":%s,"p99_ms":%s,"request_count":%s,"outbox_pending_count":%s,"seq_alloc_avg_ms":%s,"kafka_publish_avg_ms":%s,"result_file":%s}\n' \
        "$cpu" "$(json_string "$memory")" "$outbox_workers" "$vu" "$(json_bool "$passed")" \
        "$success_rate" "$p95_ms" "$p99_ms" "$request_count" "$outbox_pending_count" \
        "${seq_alloc_avg_ms:-null}" "${kafka_publish_avg_ms:-null}" "$(json_string "$host_summary_path")" >> "$matrix_path"

      if [[ "$passed" != "true" ]]; then
        echo "Stopping cpu=$cpu memory=$memory after vu=$vu failed threshold"
        break
      fi
    done

    printf '{"cpu_limit":%s,"memory_limit":%s,"outbox_workers":%s,"vus":"max_pass","passed":true,"max_passing_vus":%s}\n' \
      "$cpu" "$(json_string "$memory")" "$outbox_workers" "$max_passing_vu" >> "$matrix_path"
  done
done

echo "Docker resource matrix summary written to $matrix_path"
