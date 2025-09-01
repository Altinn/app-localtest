#!/bin/bash

# Script for comparing PDF generation perf betwee our existing (outdated) browserless/puppeteer image
# and potential new solutions. Current configuration is running
# * Google Chrome for Testing 117.0.5938.92
# * PREBOOT_CHROME=true
# * MAX_CONCURRENT_SESSIONS=4
# * (queue size = 1, but we don't hit that here so we dont care)

# TODO
# * https://github.com/spider-rs/spider_chrome/issues/1 - other chrome variants?
# * PuppeteerSharp

set -Eeuo pipefail

INSTANCE_ID="501337/9e98fe2f-87c6-4254-9172-b6923f91d7ab"

create_json_payload() {
    local url="$1"
    local token="$2"
    cat <<EOF
{
    "url": "$url",
    "options": {
        "headerTemplate": "<div/>",
        "footerTemplate": "<div/>",
        "displayHeaderFooter": false,
        "printBackground": true,
        "format": "A4",
        "margin": {
            "top": "0.75in",
            "right": "0.75in",
            "bottom": "0.75in",
            "left": "0.75in"
        }
    },
    "setJavaScriptEnabled": true,
    "waitFor": "",
    "cookies": [
        {
            "name": "AltinnStudioRuntime",
            "value": "$token",
            "domain": "local.altinn.cloud",
            "sameSite": "Lax"
        }
    ]
}
EOF
}

generate_pdf_curl() {
    local service_name="$1"
    local service_url="$2" 
    local output_file="$3"
    local url="$4"
    local token="$5"
    
    # Create filename-safe version of service name
    local safe_service_name=$(echo "$service_name" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')
    
    echo "Testing $service_name..."
    echo "======================================"
    
    # Create temporary JSON file to avoid command line length issues
    local temp_payload="/tmp/${safe_service_name}_payload.json"
    create_json_payload "$url" "$token" > "$temp_payload"
    curl -X POST \
        -H "Content-Type: application/json" \
        -d @"$temp_payload" \
        "$service_url" \
        -o "$output_file" \
        -D "output/headers-${safe_service_name}.txt"
    rm -f "$temp_payload"
    
    # Measure memory after single PDF generation
    local container_name=""
    case "$service_url" in
        *5300*) container_name="altinn-pdf-service" ;;
        *5010*) container_name="altinn-pdf-rust" ;;
        *5011*) container_name="altinn-pdf-go" ;;
    esac
    
    if [ -n "$container_name" ]; then
        local memory_after_single=$(measure_container_memory "$container_name")
        echo "$memory_after_single" > "/tmp/memory_after_single_${container_name}.txt"
        echo "Memory usage after single PDF: ${memory_after_single}MB"
    fi
    
    echo "======================================"
}

run_load_test() {
    local service_name="$1"
    local service_url="$2"
    local output_log="$3"
    local url="$4"
    local container_name="$5"
    local token="$6"
    
    # Create filename-safe version of service name
    local safe_service_name=$(echo "$service_name" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')
    
    echo "Load testing $service_name..."
    
    echo "" >> "$output_log"
    echo "Load Test Results:" >> "$output_log"
    echo "==================" >> "$output_log"
    autocannon --duration 300 -c 3 -p 3 -R 3 -l \
        --method POST \
        --headers "Content-Type: application/json" \
        --body "$(create_json_payload "$url" "$token")" \
        "$service_url" >> "$output_log" 2>&1
    
    # Measure memory after load test
    local memory_after=$(measure_container_memory "$container_name")
    echo "" >> "$output_log"
    echo "Memory usage (after load test): ${memory_after}MB" >> "$output_log"
    
    # Calculate memory difference if we have the before value
    if [ -f "/tmp/memory_before_${container_name}.txt" ]; then
        local memory_before=$(cat "/tmp/memory_before_${container_name}.txt")
        local memory_diff=$((memory_after - memory_before))
        if [ $memory_diff -gt 0 ]; then
            echo "Memory increase during load test: +${memory_diff}MB" >> "$output_log"
        elif [ $memory_diff -lt 0 ]; then
            echo "Memory decrease during load test: ${memory_diff}MB" >> "$output_log"
        else
            echo "Memory change during load test: 0MB" >> "$output_log"
        fi
        rm -f "/tmp/memory_before_${container_name}.txt"
    fi
    
    echo "" >> "$output_log"
}

measure_container_startup_time() {
    local container_name="$1"
    
    # Get container ID and start time
    local container_id=$(docker inspect -f '{{.Id}}' "$container_name" 2>/dev/null)
    if [ -z "$container_id" ]; then
        echo "0" # Container not found
        return
    fi
    
    # Get container start time in RFC-3339 format using .Created
    local start_time_rfc=$(docker inspect -f '{{.Created}}' "$container_name" 2>/dev/null)
    if [ -z "$start_time_rfc" ]; then
        echo "0" # Could not get start time
        return
    fi
    
    # Convert start time to milliseconds (using nanoseconds precision)
    local start_ms=$(date -d "$start_time_rfc" +%s%3N 2>/dev/null)
    if [ -z "$start_ms" ] || [ "$start_ms" -eq 0 ]; then
        echo "0" # Could not parse start time
        return
    fi
    
    # Use docker events to find the first healthy event
    local healthy_event=$(
        docker events \
            --since "$start_time_rfc" \
            --until "$(date -u -Is)" \
            --filter "container=$container_id" \
            --filter "event=health_status" \
            --format '{{.TimeNano}} {{.Actor.Attributes.health_status}} {{.Status}}' \
        | awk '/healthy/ {print $1; exit}'
    )
    
    if [ -z "$healthy_event" ]; then
        # No healthy event found - check if container is currently healthy
        local current_health=$(docker inspect "$container_name" --format='{{.State.Health.Status}}' 2>/dev/null)
        if [ "$current_health" = "healthy" ]; then
            # Container is healthy but we missed the event, estimate based on current time
            local current_ms=$(date +%s%3N)
            local estimated_duration=$((current_ms - start_ms))
            echo "$estimated_duration"
        else
            echo "-1" # Container not healthy and no healthy event found
        fi
        return
    fi
    
    # Convert nanoseconds to milliseconds and calculate startup duration
    local healthy_ms=$((healthy_event / 1000000))
    local startup_duration=$((healthy_ms - start_ms))
    echo "$startup_duration"
}

measure_container_memory() {
    local container_name="$1"
    
    # Get memory usage directly - docker stats returns format like "465.8MiB / 60.46GiB"
    local memory_usage=$(docker stats "$container_name" --no-stream --format "{{.MemUsage}}" 2>/dev/null | cut -d'/' -f1 | tr -d ' ')
    if [ -z "$memory_usage" ]; then
        echo "0"
        return
    fi
    
    # Extract numeric value and unit
    local memory_value=$(echo "$memory_usage" | sed 's/[A-Za-z]*$//')
    local memory_unit=$(echo "$memory_usage" | sed 's/^[0-9.]*//') 
    
    # Convert to MB based on unit
    case "$memory_unit" in
        "GiB")
            echo "$((${memory_value%.*} * 1024))"
            ;;
        "MiB")
            echo "${memory_value%.*}"
            ;;
        "KiB")
            echo "$((${memory_value%.*} / 1024))"
            ;;
        "B")
            echo "$((${memory_value%.*} / 1024 / 1024))"
            ;;
        *)
            # Default assume MiB if no clear unit
            echo "${memory_value%.*}"
            ;;
    esac
}

measure_image_size() {
    local container_name="$1"
    
    # Get image name from container
    local image_name=$(docker inspect "$container_name" --format='{{.Config.Image}}' 2>/dev/null)
    if [ -z "$image_name" ]; then
        echo "0"
        return
    fi
    
    # Get image size in MB
    local size_bytes=$(docker images "$image_name" --format "{{.Size}}" 2>/dev/null | head -1)
    if [ -z "$size_bytes" ]; then
        echo "0"
        return
    fi
    
    # Convert size to MB (docker images shows human readable format)
    if echo "$size_bytes" | grep -q "GB"; then
        local size_gb=$(echo "$size_bytes" | sed 's/GB//')
        echo "$((${size_gb%.*} * 1024))"
    elif echo "$size_bytes" | grep -q "MB"; then
        echo "${size_bytes%MB*}"
    else
        # Assume bytes, convert to MB
        echo "$((size_bytes / 1024 / 1024))"
    fi
}

collect_service_metrics() {
    local service_name="$1"
    local container_name="$2"
    local health_url="$3"
    local output_log="$4"
    local startup_time="$5"
    
    echo "" >> "$output_log"
    echo "Performance Metrics:" >> "$output_log"
    echo "===================" >> "$output_log"
    
    # Measure container image size
    local image_size=$(measure_image_size "$container_name")
    echo "Container image size: ${image_size}MB" >> "$output_log"
    
    # Use pre-measured startup time
    if [ "$startup_time" = "-1" ]; then
        echo "Startup time: TIMEOUT (>120s)" >> "$output_log"
    elif [ "$startup_time" = "0" ]; then
        echo "Startup time: Container not found" >> "$output_log"
    else
        echo "Average startup time: ${startup_time}ms (10 iterations)" >> "$output_log"
    fi
    
    # Read memory before value that was stored earlier
    local memory_before=0
    if [ -f "/tmp/memory_before_${container_name}.txt" ]; then
        memory_before=$(cat "/tmp/memory_before_${container_name}.txt")
    fi
    echo "Memory usage (before load test): ${memory_before}MB" >> "$output_log"
    
    # Read memory after single PDF generation
    local memory_after_single=0
    if [ -f "/tmp/memory_after_single_${container_name}.txt" ]; then
        memory_after_single=$(cat "/tmp/memory_after_single_${container_name}.txt")
    fi
    echo "Memory usage (after single PDF): ${memory_after_single}MB" >> "$output_log"
    
    # Calculate memory difference from single PDF generation
    if [ "$memory_after_single" -gt 0 ] && [ "$memory_before" -gt 0 ]; then
        local memory_diff_single=$((memory_after_single - memory_before))
        if [ $memory_diff_single -gt 0 ]; then
            echo "Memory increase from single PDF: +${memory_diff_single}MB" >> "$output_log"
        elif [ $memory_diff_single -lt 0 ]; then
            echo "Memory decrease from single PDF: ${memory_diff_single}MB" >> "$output_log"
        else
            echo "Memory change from single PDF: 0MB" >> "$output_log"
        fi
    fi
    
    echo "" >> "$output_log"
}

log_service_headers() {
    local service_name="$1"
    local output_log="$2"
    local safe_service_name=$(echo "$service_name" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')
    
    echo "$service_name Headers:" > "$output_log"
    echo "$(printf '%*s' ${#service_name} '' | tr ' ' '=')=========" >> "$output_log"
    cat "output/headers-${safe_service_name}.txt" >> "$output_log" 2>/dev/null || echo "No headers file found" >> "$output_log"
    echo "" >> "$output_log"
}

wait_for_health_endpoint() {
    local health_url="$1"
    local timeout_seconds="${2:-120}"
    local start_time=$(date +%s)
    
    while true; do
        if curl -s -f "$health_url" >/dev/null 2>&1; then
            return 0
        fi
        
        local current_time=$(date +%s)
        if [ $((current_time - start_time)) -ge $timeout_seconds ]; then
            return 1
        fi
        
        sleep 0.1
    done
}

measure_average_startup_time() {
    local container_name="$1"
    local service_name="$2"
    local health_url="$3"
    local iterations=10
    local total_time=0
    local valid_measurements=0
    
    echo "Measuring startup time for $container_name ($iterations iterations)..." >&2
    
    for i in $(seq 1 $iterations); do
        # Stop the specific container using service name
        if ! docker compose stop "$service_name" >/dev/null 2>&1; then
            echo "  Failed to stop service $service_name" >&2
            continue
        fi
        if ! docker compose rm -f "$service_name" >/dev/null 2>&1; then
            echo "  Failed to remove service $service_name" >&2
            continue
        fi
        
        # Record start time before starting container
        local start_time=$(date +%s%3N)
        
        # Start the container without waiting
        if ! docker compose up "$service_name" -d >/dev/null 2>&1; then
            echo "  Failed to start service $service_name" >&2
            continue
        fi
        
        # Wait for health endpoint to respond
        if wait_for_health_endpoint "$health_url" 120; then
            # Record end time after health endpoint responds
            local end_time=$(date +%s%3N)
            
            # Calculate startup time
            local startup_time=$((end_time - start_time))
            
            if [ "$startup_time" -gt 0 ]; then
                total_time=$((total_time + startup_time))
                valid_measurements=$((valid_measurements + 1))
            fi
        else
            echo "  Health endpoint timeout for $service_name" >&2
        fi
    done
    
    if [ "$valid_measurements" -gt 0 ]; then
        local average_time=$((total_time / valid_measurements))
        echo "  $container_name: ${average_time}ms avg (${valid_measurements}/$iterations)" >&2
        echo "$average_time"
    else
        echo "  $container_name: No valid measurements" >&2
        echo "0"
    fi

    if ! docker compose stop "$service_name" >/dev/null 2>&1; then
        echo "  Failed to stop service $service_name" >&2
        continue
    fi
    if ! docker compose rm -f "$service_name" >/dev/null 2>&1; then
        echo "  Failed to remove service $service_name" >&2
        continue
    fi
}

execute() {
    local url="$1"
    
    # Measure average startup times with multiple iterations
    echo "Measuring startup times..."
    # local browserless_startup=$(measure_average_startup_time "altinn-pdf-service" "altinn_pdf_service" "http://127.0.0.1:5300/json")
    # local rust_startup=$(measure_average_startup_time "altinn-pdf-rust" "pdf_rust" "http://127.0.0.1:5010/health")
    local go_startup=$(measure_average_startup_time "altinn-pdf-go" "pdf_go" "http://127.0.0.1:5011/health")
    
    # Ensure all containers are running after measurements
    echo "Ensuring all containers are running..."
    docker compose up -d --wait
    
    # Measure memory before any PDF generation
    echo "Measuring baseline memory usage..."
    # local browserless_memory_before=$(measure_container_memory "altinn-pdf-service")
    # local rust_memory_before=$(measure_container_memory "altinn-pdf-rust")
    local go_memory_before=$(measure_container_memory "altinn-pdf-go")
    
    # Store memory before values for later use
    # echo "$browserless_memory_before" > "/tmp/memory_before_altinn-pdf-service.txt"
    # echo "$rust_memory_before" > "/tmp/memory_before_altinn-pdf-rust.txt"
    echo "$go_memory_before" > "/tmp/memory_before_altinn-pdf-go.txt"
    
    # Get token (containers are ready due to --wait)
    local token=$(curl -s "http://local.altinn.cloud/Home/GetTestOrgToken/ttd?orgNumber=405003309&scopes=altinn:serviceowner/instances.read%20altinn:serviceowner/instances.write" || echo "")
    
    if [ -z "$token" ]; then
        echo "Warning: Could not get token"
        token=""
    fi
    
    # Always run curl commands first for PDF inspection
    # generate_pdf_curl "browserless container" "http://127.0.0.1:5300/pdf" "output/test-browserless.pdf" "$url" "$token"
    # generate_pdf_curl "rust PDF service" "http://127.0.0.1:5010/pdf" "output/test-rust.pdf" "$url" "$token"
    generate_pdf_curl "go PDF service" "http://127.0.0.1:5011/pdf" "output/test-go.pdf" "$url" "$token"

    echo "PDFs saved to output/test-browserless.pdf, output/test-rust.pdf, and output/test-go.pdf"
    
    # Extract and log headers from all services
    # log_service_headers "Browserless Container" "output/result-browserless.log"
    # log_service_headers "Rust PDF Service" "output/result-rust.log"
    log_service_headers "Go PDF Service" "output/result-go.log"
    
    # Collect performance metrics for all services
    echo ""
    echo "======================================"
    echo "Collecting performance metrics..."
    echo "======================================"
    # collect_service_metrics "Browserless Container" "altinn-pdf-service" "http://127.0.0.1:5300/json" "output/result-browserless.log" "$browserless_startup"
    # collect_service_metrics "Rust PDF Service" "altinn-pdf-rust" "http://127.0.0.1:5010/health" "output/result-rust.log" "$rust_startup"
    collect_service_metrics "Go PDF Service" "altinn-pdf-go" "http://127.0.0.1:5011/health" "output/result-go.log" "$go_startup"
    
    # If not in test mode, also run load tests
    if [ "${TEST:-}" != "1" ]; then
        echo ""
        echo "======================================"
        echo "Starting load tests..."
        echo "======================================"
        
        # run_load_test "browserless container" "http://127.0.0.1:5300/pdf" "output/result-browserless.log" "$url" "altinn-pdf-service" "$token"
        # echo "--------------------------------------"
        # run_load_test "rust PDF service" "http://127.0.0.1:5010/pdf" "output/result-rust.log" "$url" "altinn-pdf-rust" "$token"
        # echo "--------------------------------------"
        run_load_test "go PDF service" "http://127.0.0.1:5011/pdf" "output/result-go.log" "$url" "altinn-pdf-go" "$token"
        
        echo "Load test results saved to output/result-browserless.log, output/result-rust.log, and output/result-go.log"
    fi
}

main() {
    # Build images first, then clean slate
    docker compose build
    docker compose down

    echo ""
    echo ""

    # Direct browserless container call
    echo "${TEST:+Test mode - }${TEST:-Load testing }PDF generation..."
    # Construct URL the same way PdfService does
    URL="http://local.altinn.cloud/ttd/subform-test/#/instance/${INSTANCE_ID}?pdf=1&lang=nb"
    execute "$URL"
}

# Only run main if script is executed directly (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main
fi
