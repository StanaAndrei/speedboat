package orchestrator

import (
	"fmt"
	"strings"
)

func (o *Orchestrator) isIPBlacklisted(ip string) bool {
	if ip == "" {
		return false
	}
	o.blacklistMu.RLock()
	defer o.blacklistMu.RUnlock()
	return o.blacklistedIPs[ip]
}

func (o *Orchestrator) blacklistIP(ip, reason string, tasksMap ...map[string]*managedTask) {
	if ip == "" {
		return
	}
	o.blacklistMu.Lock()
	if o.blacklistedIPs == nil {
		o.blacklistedIPs = make(map[string]bool)
	}
	already := o.blacklistedIPs[ip]
	o.blacklistedIPs[ip] = true
	o.blacklistMu.Unlock()

	if len(tasksMap) > 0 && tasksMap[0] != nil {
		for _, mt := range tasksMap[0] {
			delete(mt.assignedIPs, ip)
		}
	}

	if !already {
		fmt.Printf("🚨 [Master] Blacklisted IP %s: %s\n", ip, reason)
		if o.Store != nil {
			if err := o.Store.BlacklistIP(ip, reason); err != nil {
				fmt.Printf("⚠️  [Master] Failed to persist blacklisted IP %s: %v\n", ip, err)
			}
		}
	}
}

func getWorkerIP(resIP, workerID string) string {
	if resIP != "" {
		return resIP
	}
	parts := strings.Split(workerID, "-")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return workerID
}
