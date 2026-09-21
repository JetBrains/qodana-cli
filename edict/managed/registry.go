// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

// Package managed owns Edict's persisted state and capability-based skill execution.
package managed

import (
	"fmt"
	"slices"
)

// Policy separates the authority a skill can delegate from operations it can execute.
// Registration is trusted startup configuration, never an MCP operation.
type Policy struct {
	Name       string   `json:"name"`
	Operations []string `json:"operations"`
	Writes     []string `json:"writes"`
	Delegates  []string `json:"delegates"`
}

// DefaultRegistry is deliberately closed: unmanaged skills receive no capabilities.
func DefaultRegistry() []Policy {
	all := []string{"inbox.write", "inbox.delete", "cluster.write", "cluster.signal.write", "example.write", "inspection.write"}
	generation := []string{"cluster.write", "cluster.signal.write", "example.write", "inspection.write"}
	run := append(slices.Clone(generation), "inbox.delete")
	return []Policy{
		{Name: "edict_manager", Operations: all, Delegates: []string{"edict-next-run", "edict-next-batch-signal-analysis"}},
		{Name: "edict-next-run", Operations: run, Delegates: []string{"edict-next-prepare", "edict-next-distribution", "edict-next-generation"}},
		{Name: "edict-next-prepare"},
		{Name: "edict-next-distribution", Operations: []string{"inbox.delete", "cluster.write", "cluster.signal.write"}, Writes: []string{"inbox.delete", "cluster.write", "cluster.signal.write"}},
		{Name: "edict-next-generation", Operations: generation, Delegates: []string{"edict-next-cluster-generation"}},
		{Name: "edict-next-cluster-generation", Operations: generation, Writes: []string{"cluster.write", "inspection.write"}, Delegates: []string{"edict-next-code-example", "edict-next-inspection-code-review", "edict-next-weak-signal-review", "edict-next-inspection-value-review"}},
		{Name: "edict-next-code-example", Operations: []string{"example.write", "cluster.signal.write"}, Writes: []string{"example.write", "cluster.signal.write"}},
		{Name: "edict-next-inspection-code-review"},
		{Name: "edict-next-inspection-value-review"},
		{Name: "edict-next-weak-signal-review", Operations: []string{"example.write", "cluster.signal.write"}, Delegates: []string{"edict-next-code-example"}},
		{Name: "edict-next-batch-signal-analysis", Operations: []string{"inbox.write"}, Writes: []string{"inbox.write"}, Delegates: []string{"edict-next-signal-analysis"}},
		{Name: "edict-next-signal-analysis"},
	}
}

func register(policies []Policy) (map[string]Policy, error) {
	result := make(map[string]Policy, len(policies))
	for _, policy := range policies {
		if policy.Name == "" || result[policy.Name].Name != "" {
			return nil, fmt.Errorf("empty or duplicate skill registration: %q", policy.Name)
		}
		for _, op := range policy.Writes {
			if !slices.Contains(policy.Operations, op) {
				return nil, fmt.Errorf("%s cannot execute unregistered operation %s", policy.Name, op)
			}
		}
		result[policy.Name] = policy
	}
	for _, policy := range policies {
		for _, child := range policy.Delegates {
			if _, ok := result[child]; !ok {
				return nil, fmt.Errorf("%s delegates to unknown skill %s", policy.Name, child)
			}
		}
	}
	return result, nil
}
