package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// KimiUsagesResponse is the JSON shape from GET /coding/v1/usages
// (documented by kimi-cli's /usage command; same endpoint used by kimi-code).
type KimiUsagesResponse struct {
	User     *KimiUser            `json:"user"`
	Usage    *KimiUsageDetail     `json:"usage"`
	Limits   []KimiWindowLimit    `json:"limits"`
	Parallel *KimiParallel        `json:"parallel"`
	Total    *KimiUsageDetail     `json:"totalQuota"`
	Auth     *KimiAuthentication  `json:"authentication"`
	SubType  string               `json:"subType"`
}

// KimiUser holds identity/membership metadata from the usages endpoint.
type KimiUser struct {
	UserID       string            `json:"userId"`
	Region       string            `json:"region"`
	BusinessID   string            `json:"businessId"`
	Membership   *KimiMembership   `json:"membership"`
}

// KimiMembership holds plan level (e.g. LEVEL_INTERMEDIATE).
type KimiMembership struct {
	Level string `json:"level"`
}

// KimiUsageDetail is a limit/used/remaining block. Numeric fields often arrive as strings.
type KimiUsageDetail struct {
	Limit     json.Number `json:"limit"`
	Used      json.Number `json:"used"`
	Remaining json.Number `json:"remaining"`
	ResetTime string      `json:"resetTime"`
}

// KimiWindowLimit is a time-windowed rate limit (e.g. 300 minutes → 5h).
type KimiWindowLimit struct {
	Window *KimiWindow     `json:"window"`
	Detail *KimiUsageDetail `json:"detail"`
}

// KimiWindow describes a duration window for a limit bucket.
type KimiWindow struct {
	Duration int    `json:"duration"`
	TimeUnit string `json:"timeUnit"`
}

// KimiParallel holds concurrent session limits.
type KimiParallel struct {
	Limit json.Number `json:"limit"`
}

// KimiAuthentication describes how the token was accepted.
type KimiAuthentication struct {
	Method string `json:"method"`
	Scope  string `json:"scope"`
}

// KimiQuota is one normalized quota for storage and UI (utilization is 0-100 used%).
type KimiQuota struct {
	Name        string
	Utilization float64
	ResetsAt    *time.Time
	Status      string
	// Optional raw counts for future display (not required by renderer).
	Limit     float64
	Used      float64
	Remaining float64
}

// KimiSnapshot is the storage + UI representation of one poll.
type KimiSnapshot struct {
	ID         int64
	CapturedAt time.Time
	AccountID  int64
	UserID     string
	Region     string
	Membership string
	Quotas     []KimiQuota
	RawJSON    string
}

const (
	KimiQuotaWeekly = "weekly"
	KimiQuota5h     = "5h"
	KimiQuotaTotal  = "total"
)

var kimiDisplayNames = map[string]string{
	KimiQuotaWeekly: "Weekly",
	KimiQuota5h:     "5h Limit",
	KimiQuotaTotal:  "Total Quota",
}

// KimiDisplayName returns a UI label for a kimi quota key.
func KimiDisplayName(key string) string {
	if name, ok := kimiDisplayNames[key]; ok {
		return name
	}
	return key
}

func kimiStatusFromUtilization(util float64) string {
	switch {
	case util >= 95:
		return "critical"
	case util >= 80:
		return "danger"
	case util >= 50:
		return "warning"
	default:
		return "healthy"
	}
}

func parseKimiNumber(n json.Number) (float64, bool) {
	if n == "" {
		return 0, false
	}
	f, err := n.Float64()
	if err != nil {
		// try integer string edge cases
		i, err2 := strconv.ParseInt(string(n), 10, 64)
		if err2 != nil {
			return 0, false
		}
		return float64(i), true
	}
	return f, true
}

func parseKimiResetTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Truncate sub-microsecond fractions for time.Parse compatibility.
	if dot := strings.Index(s, "."); dot >= 0 && strings.HasSuffix(s, "Z") {
		frac := s[dot+1 : len(s)-1]
		if len(frac) > 9 {
			frac = frac[:9]
		}
		if len(frac) > 6 {
			// keep nanos if parseable as RFC3339Nano
			s = s[:dot+1] + frac + "Z"
		} else {
			for len(frac) < 6 {
				frac += "0"
			}
			s = s[:dot+1] + frac + "Z"
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

// utilizationFromDetail returns used% (0-100). Prefer used/limit; fall back to (limit-remaining)/limit.
func utilizationFromDetail(d *KimiUsageDetail) (util, limit, used, remaining float64, resets *time.Time, ok bool) {
	if d == nil {
		return 0, 0, 0, 0, nil, false
	}
	limit, hasLimit := parseKimiNumber(d.Limit)
	used, hasUsed := parseKimiNumber(d.Used)
	remaining, hasRem := parseKimiNumber(d.Remaining)
	resets = parseKimiResetTime(d.ResetTime)

	if !hasLimit && !hasUsed && !hasRem {
		return 0, 0, 0, 0, resets, false
	}
	if hasLimit && limit > 0 {
		if !hasUsed && hasRem {
			used = limit - remaining
			if used < 0 {
				used = 0
			}
		}
		util = used / limit * 100.0
		if util > 100 {
			util = 100
		}
		if util < 0 {
			util = 0
		}
		if !hasRem {
			remaining = limit - used
		}
		return util, limit, used, remaining, resets, true
	}
	// no usable limit — still surface used if present as 0 util marker
	return 0, limit, used, remaining, resets, hasUsed || hasRem
}

func windowLabel(w *KimiWindow, idx int) string {
	if w == nil {
		return fmt.Sprintf("limit_%d", idx+1)
	}
	unit := strings.ToUpper(w.TimeUnit)
	d := w.Duration
	if strings.Contains(unit, "MINUTE") {
		if d >= 60 && d%60 == 0 {
			hours := d / 60
			if hours == 5 {
				return KimiQuota5h
			}
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dm", d)
	}
	if strings.Contains(unit, "HOUR") {
		if d == 5 {
			return KimiQuota5h
		}
		return fmt.Sprintf("%dh", d)
	}
	if strings.Contains(unit, "DAY") {
		return fmt.Sprintf("%dd", d)
	}
	return fmt.Sprintf("window_%d", idx+1)
}

// ToSnapshot converts a usages API response into a KimiSnapshot.
func (r *KimiUsagesResponse) ToSnapshot(capturedAt time.Time) *KimiSnapshot {
	snap := &KimiSnapshot{
		CapturedAt: capturedAt.UTC(),
		AccountID:  1,
	}
	if r.User != nil {
		snap.UserID = r.User.UserID
		snap.Region = r.User.Region
		if r.User.Membership != nil {
			snap.Membership = r.User.Membership.Level
		}
	}
	if raw, err := json.Marshal(r); err == nil {
		snap.RawJSON = string(raw)
	}

	// Primary weekly usage
	if util, limit, used, rem, resets, ok := utilizationFromDetail(r.Usage); ok {
		snap.Quotas = append(snap.Quotas, KimiQuota{
			Name:        KimiQuotaWeekly,
			Utilization: util,
			ResetsAt:    resets,
			Status:      kimiStatusFromUtilization(util),
			Limit:       limit,
			Used:        used,
			Remaining:   rem,
		})
	}

	// Window limits (e.g. 5h)
	for i, lim := range r.Limits {
		name := windowLabel(lim.Window, i)
		// avoid colliding with weekly/total names
		if name == KimiQuotaWeekly || name == KimiQuotaTotal {
			name = fmt.Sprintf("%s_%d", name, i+1)
		}
		if util, limit, used, rem, resets, ok := utilizationFromDetail(lim.Detail); ok {
			snap.Quotas = append(snap.Quotas, KimiQuota{
				Name:        name,
				Utilization: util,
				ResetsAt:    resets,
				Status:      kimiStatusFromUtilization(util),
				Limit:       limit,
				Used:        used,
				Remaining:   rem,
			})
		}
	}

	// totalQuota (optional third card)
	if util, limit, used, rem, resets, ok := utilizationFromDetail(r.Total); ok {
		// total may report remaining only — still useful
		snap.Quotas = append(snap.Quotas, KimiQuota{
			Name:        KimiQuotaTotal,
			Utilization: util,
			ResetsAt:    resets,
			Status:      kimiStatusFromUtilization(util),
			Limit:       limit,
			Used:        used,
			Remaining:   rem,
		})
	}

	return snap
}
