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

From `/usages` (same parsing as official kimi-code / kimi-cli):

| Card | Source | Meaning |
|------|--------|---------|
| **7-day** | `usage` | 7-day utilization (`used/limit`). Product UI may show one decimal place; the API usually returns integer percents. |
| **5-hour** | `limits[]` with `duration=300` + `TIME_UNIT_MINUTE` | Rolling 5-hour window |
| Other windows | other `limits[]` entries | Labeled from window duration (e.g. `Nh limit`) |

**Not available from `/usages` today:** a long-horizon “total usage” meter with a multi-week reset (e.g. monthly). Official clients also ignore `totalQuota` unless a `boosterWallet` block is present. If you see total usage only in the Kimi website UI, that data is not exposed on this endpoint yet.

Membership (`user.membership.level`) is shown in Insights when present.

### Timezones

`resetTime` values are UTC. The dashboard formats them in your configured timezone (Settings). Example: `2026-07-14T16:13:41Z` → `2026-07-15 00:13` in Asia/Shanghai.

## Verify

```bash
# slash command in legacy kimi-cli (same API)
kimi-cli  # then /usage

# or curl after refreshing a token
curl -sS -H "Authorization: Bearer $TOKEN" https://api.kimi.com/coding/v1/usages | jq .
```

Restart onWatch and open the **Kimi Code** dashboard tab.
