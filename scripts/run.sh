#!/usr/bin/env sh

set -e

show_help() {
    cat << EOF
Usage: $(basename "$0") [OPTIONS] [COMMAND]

Run localtest with optional monitoring stack.

OPTIONS:
    -m              Enable monitoring stack (Grafana, Tempo, Mimir, Loki)
    -p              Force podman mode (use podman even if docker is installed)
    -h, --help      Show this help message

COMMANDS:
    (none), start, up    Start localtest
    stop, down           Stop localtest and monitoring services
    k6                   Run k6 load test

EXAMPLES:
    $(basename "$0")              Start localtest
    $(basename "$0") -m           Start localtest with monitoring
    $(basename "$0") -p           Start localtest using podman
    $(basename "$0") -m -p        Start localtest with monitoring using podman
    $(basename "$0") stop         Stop all services
    $(basename "$0") k6           Run k6 load test

EOF
}

include_monitoring=false
force_podman=false

# Parse all arguments, including those after commands
while [ $# -gt 0 ]; do
    case "$1" in
        -m) include_monitoring=true; shift ;;
        -p) force_podman=true; shift ;;
        -h|--help) show_help; exit 0 ;;
        stop|down) command="stop"; shift ;;
        start|up) command="start"; shift ;;
        k6) command="k6"; shift ;;
        -*) echo "Unknown option: $1" >&2; exit 1 ;;
        *) shift ;;
    esac
done

# Determine which container runtime to use
if [ "$force_podman" = false ] && [ -x "$(command -v docker)" ]; then
    cmd="docker"
    base="docker"
elif [ -x "$(command -v podman)" ]; then
    cmd="podman"
    base="podman"
else
    echo "Preqreqs missing - please install docker or podman"
    exit 1
fi

# Set monitoring file based on flag OR if monitoring is already running
monitoring_file=""
if [ "$include_monitoring" = true ] || $cmd ps --format '{{.Names}}' | grep -q '^monitoring_'; then
    monitoring_file="--file ${base}-compose.monitoring.yml"
fi

if [ "$command" = "stop" ]; then
    echo "Stopping localtest!"
    echo "Stopping using $cmd"
    $cmd compose --file ${base}-compose.yml $monitoring_file down -v
elif [ "$command" = "k6" ]; then
    echo "Running k6 loadtest!"
    cmd="podman"
    if [ "$force_podman" = false ] && [ -x "$(command -v docker)" ]; then
        cmd="docker"
    fi
    $cmd pull grafana/k6:master-with-browser
    $cmd run --rm -i --net=host grafana/k6:master-with-browser run - <k6/loadtest.js
else
    echo "Running localtest!"
    echo "Running using $cmd"
    $cmd compose --file ${base}-compose.yml $monitoring_file down -v
    $cmd compose --file ${base}-compose.yml $monitoring_file up -d --build
fi
