# Kimi Code Setup

onWatch tracks **Kimi Code** (the coding agent OAuth product) quotas via:

```http
GET https://api.kimi.com/coding/v1/usages
Authorization: Bearer <access_token>
```

This is **not** the Moonshot Open Platform pay-as-you-go balance API (`api.moonshot.ai` / `api.moonshot.cn`).

## Prerequisites

1. Install and log in with [Kimi Code](https://moonshotai.github.io/kimi-code/) (`kimi login`), **or** the legacy `kimi-cli` with Kimi Code OAuth.
2. Credentials are stored at:
   - `~/.kimi-code/credentials/kimi-code.json` (preferred)
   - `~/.kimi/credentials/kimi-code.json` (legacy, pre-migration)

## Enable

Auto-detect is on by default when credentials exist:

```bash
# optional explicit enable
KIMI_CODE_ENABLED=true

# optional disable
KIMI_CODE_ENABLED=false
```

Docker / CI without local files:

```bash
KIMI_TOKEN=<access_token>
# or
KIMI_CODE_TOKEN=<access_token>
```

For long-running daemons, prefer mounting the credentials file and allowing refresh:

```bash
KIMI_CODE_CREDENTIALS=/path/to/kimi-code.json
```

onWatch will refresh expired access tokens via:

```http
POST https://auth.kimi.com/api/oauth/token
grant_type=refresh_token&refresh_token=...&client_id=17e5f671-d194-4dfb-9706-5516cb48c098
```

and rewrite the credentials file (mode `0600`).

## What is tracked

From `/usages`:

| Card | Source | Meaning |
|------|--------|---------|
| Weekly | `usage` | Primary weekly utilization (`used/limit`) |
| 5h Limit | `limits[]` window (300 minutes) | Short window quota |
| Total Quota | `totalQuota` | Additional total remaining/limit |

Membership (`user.membership.level`) is shown in Insights when present.

## Verify

```bash
# slash command in legacy kimi-cli (same API)
kimi-cli  # then /usage

# or curl after refreshing a token
curl -sS -H "Authorization: Bearer $TOKEN" https://api.kimi.com/coding/v1/usages | jq .
```

Restart onWatch and open the **Kimi Code** dashboard tab.
