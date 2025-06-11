#!/bin/bash

# RSS Feed Registration Script
# Usage: ./register_rss_feed.sh <rss_feed_url> [base_url]

set -e

# Default configuration
DEFAULT_BASE_URL="http://localhost:9000"
API_ENDPOINT="/v1/rss-feed-link/register"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to display usage
show_usage() {
    echo -e "${BLUE}RSS Feed Registration Script${NC}"
    echo ""
    echo "Usage: $0 <rss_feed_url> [base_url]"
    echo ""
    echo "Arguments:"
    echo "  rss_feed_url    The RSS feed URL to register (must start with https://)"
    echo "  base_url        Optional. API base URL (default: $DEFAULT_BASE_URL)"
    echo ""
    echo "Examples:"
    echo "  $0 https://example.com/feed.xml"
    echo "  $0 https://example.com/feed.xml http://localhost:3000"
    echo "  $0 https://feeds.feedburner.com/example https://api.myserver.com"
    echo ""
}

# Function to validate URL
validate_url() {
    local url="$1"

    if [[ -z "$url" ]]; then
        echo -e "${RED}Error: RSS feed URL is required${NC}"
        return 1
    fi

    if [[ ! "$url" =~ ^https:// ]]; then
        echo -e "${RED}Error: URL must start with https://${NC}"
        return 1
    fi

    # Basic URL format validation
    if [[ ! "$url" =~ ^https://[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(/.*)?$ ]]; then
        echo -e "${YELLOW}Warning: URL format might be invalid${NC}"
    fi

    return 0
}

# Function to register RSS feed
register_feed() {
    local feed_url="$1"
    local base_url="$2"
    local full_url="${base_url}${API_ENDPOINT}"

    echo -e "${BLUE}Registering RSS feed...${NC}"
    echo "Feed URL: $feed_url"
    echo "API URL: $full_url"
    echo ""

    # Create JSON payload
    local json_payload=$(cat <<EOF
{
    "url": "$feed_url"
}
EOF
)

    # Make the API request
    local response
    local http_code

    response=$(curl -s -w "\n%{http_code}" \
        -X POST \
        -H "Content-Type: application/json" \
        -H "Accept: application/json" \
        -d "$json_payload" \
        "$full_url" 2>/dev/null)

    # Extract HTTP status code (last line) and response body (everything else)
    http_code=$(echo "$response" | tail -n1)
    response_body=$(echo "$response" | head -n -1)

    # Handle the response
    case "$http_code" in
        200)
            echo -e "${GREEN}✓ Success!${NC} RSS feed registered successfully"
            if [[ -n "$response_body" ]]; then
                echo "Response: $response_body"
            fi
            ;;
        400)
            echo -e "${RED}✗ Bad Request (400)${NC}"
            if [[ -n "$response_body" ]]; then
                echo "Error details: $response_body"
            fi
            exit 1
            ;;
        500)
            echo -e "${RED}✗ Internal Server Error (500)${NC}"
            if [[ -n "$response_body" ]]; then
                echo "Error details: $response_body"
            fi
            exit 1
            ;;
        000)
            echo -e "${RED}✗ Connection Error${NC}"
            echo "Could not connect to the API server at: $full_url"
            echo "Please check if the server is running and the URL is correct."
            exit 1
            ;;
        *)
            echo -e "${RED}✗ Unexpected Response (HTTP $http_code)${NC}"
            if [[ -n "$response_body" ]]; then
                echo "Response: $response_body"
            fi
            exit 1
            ;;
    esac
}

# Main script logic
main() {
    # Check if help is requested
    if [[ "$1" == "-h" || "$1" == "--help" ]]; then
        show_usage
        exit 0
    fi

    # Check if curl is available
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}Error: curl is required but not installed${NC}"
        echo "Please install curl to use this script"
        exit 1
    fi

    # Parse arguments
    local feed_url="$1"
    local base_url="${2:-$DEFAULT_BASE_URL}"

    # Validate inputs
    if ! validate_url "$feed_url"; then
        echo ""
        show_usage
        exit 1
    fi

    # Remove trailing slash from base URL if present
    base_url="${base_url%/}"

    # Register the feed
    register_feed "$feed_url" "$base_url"
}

# Run the main function with all arguments
main "$@"