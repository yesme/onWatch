package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestKimiUsagesToSnapshot(t *testing.T) {
	raw := []byte(`{
	  "user": {"userId": "u1", "region": "REGION_CN", "membership": {"level": "LEVEL_INTERMEDIATE"}},
	  "usage": {"limit": "100", "used": "66", "remaining": "34", "resetTime": "2026-07-15T07:13:41.897674Z"},
	  "limits": [{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"}, "detail": {"limit": "100", "remaining": "100", "resetTime": "2026-07-14T16:13:41Z"}}],
	  "totalQuota": {"limit": "100", "remaining": "99"}
	}`)
	var resp KimiUsagesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	snap := resp.ToSnapshot(time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC))
	if snap.UserID != "u1" {
		t.Fatalf("user id: %s", snap.UserID)
	}
	if snap.Membership != "LEVEL_INTERMEDIATE" {
		t.Fatalf("membership: %s", snap.Membership)
	}
	if len(snap.Quotas) < 2 {
		t.Fatalf("expected >=2 quotas, got %d", len(snap.Quotas))
	}
	var weekly *KimiQuota
	for i := range snap.Quotas {
		if snap.Quotas[i].Name == KimiQuotaWeekly {
			weekly = &snap.Quotas[i]
		}
	}
	if weekly == nil {
		t.Fatal("missing weekly quota")
	}
	if weekly.Utilization < 65.9 || weekly.Utilization > 66.1 {
		t.Fatalf("weekly util: %v", weekly.Utilization)
	}
	if weekly.ResetsAt == nil {
		t.Fatal("expected weekly reset time")
	}
	if weekly.Status != "warning" {
		// 66% is warning (>=50)
		t.Fatalf("status: %s", weekly.Status)
	}
}

func TestKimiDisplayName(t *testing.T) {
	if KimiDisplayName(KimiQuotaWeekly) != "Weekly" {
		t.Fatal(KimiDisplayName(KimiQuotaWeekly))
	}
	if KimiDisplayName("custom") != "custom" {
		t.Fatal(KimiDisplayName("custom"))
	}
}

func TestUtilizationFromRemainingOnly(t *testing.T) {
	d := &KimiUsageDetail{Limit: "100", Remaining: "40"}
	util, limit, used, rem, _, ok := utilizationFromDetail(d)
	if !ok {
		t.Fatal("expected ok")
	}
	if limit != 100 || used != 60 || rem != 40 {
		t.Fatalf("limit=%v used=%v rem=%v", limit, used, rem)
	}
	if util < 59.9 || util > 60.1 {
		t.Fatalf("util=%v", util)
	}
}
