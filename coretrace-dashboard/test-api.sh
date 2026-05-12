#!/usr/bin/env bash
# CoreTrace Dashboard — API integration tests
#
# Usage:
#   ./test-api.sh                          # expects API at http://localhost:8080
#   ./test-api.sh http://my-host:8080
#
# Prerequisites: curl, a running dashboard stack (docker compose up -d)

set -uo pipefail

API="${1:-http://localhost:8080}"
PASS=0
FAIL=0

GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'

ok()   { echo -e "  ${GREEN}✓${NC} $1"; ((PASS++)); }
fail() { echo -e "  ${RED}✗${NC} $1"; ((FAIL++)); }

http_status() { curl -s -o /dev/null -w "%{http_code}" "$@"; }
json_field()  {
    # Extract a top-level JSON field value: json_field RESPONSE FIELD
    printf '%s' "$1" | grep -o "\"$2\":\"[^\"]*\"" | head -1 | sed "s/\"$2\"://;s/\"//g"
}
json_field_uuid() {
    # UUIDs may appear without quotes in some JSON parsers, so handle both
    printf '%s' "$1" | grep -oE "\"$2\":\"[0-9a-f-]+\"" | head -1 | sed "s/\"$2\"://;s/\"//g"
}

echo "CoreTrace API integration tests"
echo "Target: $API"
echo ""

# ── Wait for the API to be up ────────────────────────────────────────────────
echo "── Connectivity ─────────────────────────────────────────"
MAX_WAIT=15
for i in $(seq 1 $MAX_WAIT); do
    CODE=$(http_status "$API/api/v1/health" 2>/dev/null || true)
    if [ "$CODE" = "200" ]; then break; fi
    if [ "$i" = "$MAX_WAIT" ]; then
        fail "API not reachable at $API after ${MAX_WAIT}s (last HTTP code: $CODE)"
        exit 1
    fi
    sleep 1
done
ok "API is reachable"

# ── Health endpoint (public — no auth required) ───────────────────────────────
echo "── Health (public) ──────────────────────────────────────"
CODE=$(http_status "$API/api/v1/health")
[ "$CODE" = "200" ] && ok "GET /health → 200" || fail "GET /health → expected 200, got $CODE"

# ── Login ────────────────────────────────────────────────────────────────────
echo "── Authentication ───────────────────────────────────────"
LOGIN=$(curl -s -X POST "$API/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"admin@coretrace.io","password":"admin123"}')
TOKEN=$(json_field "$LOGIN" "token")

if [ -n "$TOKEN" ]; then
    ok "POST /auth/login → token received"
else
    fail "POST /auth/login → no token in response (body: $LOGIN)"
    echo "Cannot continue without a token. Exiting."
    exit 1
fi

CODE=$(http_status -X POST "$API/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"bad@example.com","password":"wrong"}')
[ "$CODE" = "401" ] && ok "Invalid credentials → 401" || fail "Invalid credentials → expected 401, got $CODE"

# ── Auth guard — unauthenticated requests ────────────────────────────────────
echo "── Auth guard (no token) ────────────────────────────────"
for ep in /agents /events /sessions /stats; do
    CODE=$(http_status "$API/api/v1$ep")
    [ "$CODE" = "401" ] \
        && ok "GET $ep without token → 401" \
        || fail "GET $ep without token → expected 401, got $CODE"
done

# ── Auth guard — authenticated requests ─────────────────────────────────────
echo "── Auth guard (with token) ──────────────────────────────"
for ep in /agents /events /sessions /stats; do
    CODE=$(http_status -H "Authorization: Bearer $TOKEN" "$API/api/v1$ep")
    [ "$CODE" = "200" ] \
        && ok "GET $ep with token → 200" \
        || fail "GET $ep with token → expected 200, got $CODE"
done

# ── Events — limit query param ───────────────────────────────────────────────
echo "── Events ?limit param ──────────────────────────────────"
AUTH="-H \"Authorization: Bearer $TOKEN\""

for limit_val in 1 5 50 100 1000; do
    CODE=$(http_status -H "Authorization: Bearer $TOKEN" "$API/api/v1/events?limit=$limit_val")
    [ "$CODE" = "200" ] \
        && ok "?limit=$limit_val → 200" \
        || fail "?limit=$limit_val → expected 200, got $CODE"
done

# Out-of-range / invalid limits should fall back to default (not 400/500)
for bad in 0 -1 1001 9999 notanumber ""; do
    CODE=$(http_status -H "Authorization: Bearer $TOKEN" "$API/api/v1/events?limit=$bad")
    [ "$CODE" = "200" ] \
        && ok "?limit=$bad (invalid) → 200 with default" \
        || fail "?limit=$bad → expected 200, got $CODE"
done

# ── Agent CRUD + API key uniqueness ──────────────────────────────────────────
echo "── Agent creation & API key uniqueness ──────────────────"
A1=$(curl -s -X POST "$API/api/v1/agents" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"test-agent-1","hostname":"host-a","ip_address":"10.0.0.10"}')
A2=$(curl -s -X POST "$API/api/v1/agents" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"test-agent-2","hostname":"host-b","ip_address":"10.0.0.11"}')

ID1=$(json_field_uuid "$A1" "id")
ID2=$(json_field_uuid "$A2" "id")

if [ -n "$ID1" ] && [ -n "$ID2" ]; then
    ok "Agent 1 created (id: $ID1)"
    ok "Agent 2 created (id: $ID2)"
    [ "$ID1" != "$ID2" ] \
        && ok "Two agents have distinct IDs" \
        || fail "Agent IDs are identical — uniqueness broken"
else
    fail "Agent creation failed (ID1='$ID1', ID2='$ID2')"
    fail "Skipping ID-uniqueness check"
fi

# ── Cleanup ───────────────────────────────────────────────────────────────────
for id in "$ID1" "$ID2"; do
    [ -n "$id" ] && curl -s -o /dev/null -X DELETE \
        -H "Authorization: Bearer $TOKEN" "$API/api/v1/agents/$id"
done

# ── Session endpoints ─────────────────────────────────────────────────────────
echo "── Sessions ─────────────────────────────────────────────"
CODE=$(http_status -H "Authorization: Bearer $TOKEN" "$API/api/v1/sessions/active")
[ "$CODE" = "200" ] \
    && ok "GET /sessions/active → 200" \
    || fail "GET /sessions/active → expected 200, got $CODE"

# ── Event stats ───────────────────────────────────────────────────────────────
echo "── Event stats ──────────────────────────────────────────"
CODE=$(http_status -H "Authorization: Bearer $TOKEN" "$API/api/v1/events/stats")
[ "$CODE" = "200" ] \
    && ok "GET /events/stats → 200" \
    || fail "GET /events/stats → expected 200, got $CODE"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "─────────────────────────────────────────────────────────"
printf "  Passed: ${GREEN}%d${NC}   Failed: ${RED}%d${NC}\n" "$PASS" "$FAIL"
if [ "$FAIL" -eq 0 ]; then
    echo -e "  ${GREEN}All tests passed.${NC}"
    exit 0
else
    echo -e "  ${RED}${FAIL} test(s) failed.${NC}"
    exit 1
fi
