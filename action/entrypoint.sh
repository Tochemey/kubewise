#!/usr/bin/env bash
set -euo pipefail

# KubeWise GitHub Action entrypoint
# Decodes kubeconfig, runs the appropriate simulation, captures output.

BINARY="${GITHUB_ACTION_PATH:-$(dirname "$0")}/kubectl-whatif"
REPORT_FILE="/tmp/kubewise-report.md"

# --- Set up kubeconfig ---
if [[ -n "${INPUT_KUBECONFIG:-}" ]]; then
    mkdir -p ~/.kube
    echo "${INPUT_KUBECONFIG}" | base64 -d > ~/.kube/config
    chmod 600 ~/.kube/config
    echo "Kubeconfig written to ~/.kube/config"
else
    echo "::error::kubeconfig input is required"
    exit 1
fi

# --- Build CLI if not present ---
if [[ ! -x "${BINARY}" ]]; then
    echo "Building kubectl-whatif..."
    go build -o "${BINARY}" ./cmd/kubectl-whatif/
fi

# --- Determine command and flags ---
CMD_ARGS=("--output=markdown" "--no-color")

if [[ -n "${INPUT_SCENARIO_FILE:-}" ]]; then
    # Scenario file overrides scenario type
    CMD_ARGS=("apply" "-f" "${INPUT_SCENARIO_FILE}" "${CMD_ARGS[@]}")
else
    case "${INPUT_SCENARIO}" in
        rightsize)
            CMD_ARGS=("rightsize"
                "--percentile=${INPUT_PERCENTILE}"
                "--buffer=${INPUT_BUFFER}"
                "${CMD_ARGS[@]}")
            ;;
        consolidate)
            if [[ -z "${INPUT_NODE_TYPE:-}" ]]; then
                echo "::error::node-type input is required for consolidation scenario"
                exit 1
            fi
            CMD_ARGS=("consolidate"
                "--node-type=${INPUT_NODE_TYPE}"
                "${CMD_ARGS[@]}")
            ;;
        spot)
            CMD_ARGS=("spot"
                "--min-replicas=${INPUT_MIN_REPLICAS}"
                "--discount=${INPUT_DISCOUNT}"
                "${CMD_ARGS[@]}")
            ;;
        snapshot)
            CMD_ARGS=("snapshot" "${CMD_ARGS[@]}")
            if [[ -n "${INPUT_SAVE:-}" ]]; then
                CMD_ARGS+=("--save=${INPUT_SAVE}")
            fi
            ;;
        *)
            echo "::error::Unknown scenario type: ${INPUT_SCENARIO}. Use rightsize, consolidate, spot, or snapshot."
            exit 1
            ;;
    esac
fi

# --- Run simulation ---
echo "Running: ${BINARY} ${CMD_ARGS[*]}"
"${BINARY}" "${CMD_ARGS[@]}" > "${REPORT_FILE}" 2>&1 || {
    echo "::warning::KubeWise simulation returned non-zero exit code"
    cat "${REPORT_FILE}"
}

# --- Set outputs ---
if [[ -f "${REPORT_FILE}" ]]; then
    echo "markdown<<EOF" >> "${GITHUB_OUTPUT:-/dev/null}"
    cat "${REPORT_FILE}" >> "${GITHUB_OUTPUT:-/dev/null}"
    echo "EOF" >> "${GITHUB_OUTPUT:-/dev/null}"

    cat "${REPORT_FILE}"
fi

# --- Fail on red risk if requested ---
if [[ "${INPUT_FAIL_ON_RISK:-false}" == "true" ]]; then
    if grep -qi "high" "${REPORT_FILE}" 2>/dev/null; then
        echo "::error::KubeWise detected high risk — failing the check"
        exit 1
    fi
fi

echo "KubeWise simulation complete"
