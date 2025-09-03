#!/bin/bash

set -Eeuo pipefail

# Default instance IDs - can be overridden via command line arguments
INSTANCE_ID_WITH_COOKIE="501337/4c61cc7f-a49d-4bc8-9a19-6b6fd026dc6b"
INSTANCE_ID_WITHOUT_COOKIE="510001/ba899aff-de19-49af-a4c8-cfc6a1e12f3e"

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
    "waitFor": "#readyForPrint",
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

create_json_payload_no_cookie() {
    local url="$1"
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
    "waitFor": "#readyForPrint",
    "cookies": []
}
EOF
}

test_pdf_generation() {
    local description="$1"
    local payload="$2"
    local output_file="$3"
    local service_url="$4"
    
    echo "Testing $description..."
    echo "======================================"
    
    # Create temporary JSON file
    local temp_payload="/tmp/test_payload.json"
    echo "$payload" > "$temp_payload"
    
    curl -X POST \
        -H "Content-Type: application/json" \
        -d @"$temp_payload" \
        "$service_url" \
        -o "$output_file" \
        -D "output/headers-$(basename "$output_file" .pdf).txt" \
        -w "HTTP Status: %{http_code}\nTime: %{time_total}s\n"
    
    rm -f "$temp_payload"
    echo "PDF saved to: $output_file"
    echo "======================================"
    echo ""
}

main() {
    # Create output directory if it doesn't exist
    mkdir -p output
    
    # Get token for the first request
    echo "Getting authentication token..."
    local token=$(curl -s "http://local.altinn.cloud/Home/GetTestOrgToken/ttd?orgNumber=405003309&scopes=altinn:serviceowner/instances.read%20altinn:serviceowner/instances.write" || echo "")
    
    if [ -z "$token" ]; then
        echo "Warning: Could not get token"
        token=""
    else
        echo "Token obtained successfully"
    fi
    
    echo ""
    
    # Test both generators with cookie
    local url_with_cookie="http://local.altinn.cloud/ttd/subform-test/#/instance/${INSTANCE_ID_WITH_COOKIE}?pdf=1&lang=nb"
    local payload_with_cookie=$(create_json_payload "$url_with_cookie" "$token")
    
    test_pdf_generation "PDF generation WITH cookie - gorod (Instance: $INSTANCE_ID_WITH_COOKIE)" "$payload_with_cookie" "output/test-gorod-with-cookie.pdf" "http://127.0.0.1:5012/pdf"
    test_pdf_generation "PDF generation WITH cookie - chromedp (Instance: $INSTANCE_ID_WITH_COOKIE)" "$payload_with_cookie" "output/test-chromedp-with-cookie.pdf" "http://127.0.0.1:5011/pdf"
    
    # Second test: without cookie (commented out but structure updated for both generators)
    # local url_without_cookie="http://local.altinn.cloud/ttd/subform-test/#/instance/${INSTANCE_ID_WITHOUT_COOKIE}?pdf=1&lang=nb"
    # local payload_without_cookie=$(create_json_payload_no_cookie "$url_without_cookie")
    # test_pdf_generation "PDF generation WITHOUT cookie - gorod (Instance: $INSTANCE_ID_WITHOUT_COOKIE)" "$payload_without_cookie" "output/test-gorod-without-cookie.pdf" "http://127.0.0.1:5012/pdf"
    # test_pdf_generation "PDF generation WITHOUT cookie - chromedp (Instance: $INSTANCE_ID_WITHOUT_COOKIE)" "$payload_without_cookie" "output/test-chromedp-without-cookie.pdf" "http://127.0.0.1:5011/pdf"
    
    echo "Test completed!"
    echo "Results:"
    echo "- Gorod with cookie: output/test-gorod-with-cookie.pdf"
    echo "- ChromeDP with cookie: output/test-chromedp-with-cookie.pdf"
    echo "- Headers: output/headers-test-gorod-with-cookie.txt, output/headers-test-chromedp-with-cookie.txt"
}

# Only run main if script is executed directly (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi