package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"
)

type Metric struct {
	Name           string   `yaml:"name"`
	Namespace      string   `yaml:"namespace,omitempty"`
	Subsystem      string   `yaml:"subsystem,omitempty"`
	Help           string   `yaml:"help"`
	Type           string   `yaml:"type"`
	StabilityLevel string   `yaml:"stabilityLevel"`
	Labels         []string `yaml:"labels,omitempty"`
	Buckets        []float64 `yaml:"buckets,omitempty"`
}

type MetricKey struct {
	Namespace string
	Subsystem string
	Name      string
}

func (m MetricKey) String() string {
	var parts []string
	if m.Namespace != "" {
		parts = append(parts, m.Namespace)
	}
	if m.Subsystem != "" {
		parts = append(parts, m.Subsystem)
	}
	parts = append(parts, m.Name)
	return strings.Join(parts, "_")
}

func loadMetrics(filename string) (map[MetricKey]Metric, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var metrics []Metric
	err = yaml.Unmarshal(data, &metrics)
	if err != nil {
		return nil, err
	}

	metricMap := make(map[MetricKey]Metric)
	for _, metric := range metrics {
		key := MetricKey{
			Namespace: metric.Namespace,
			Subsystem: metric.Subsystem,
			Name:      metric.Name,
		}
		metricMap[key] = metric
	}

	return metricMap, nil
}

type DiffType string

const (
	Added    DiffType = "Added"
	Removed  DiffType = "Removed"
	Modified DiffType = "Modified"
)

type MetricDiff struct {
	Key        MetricKey
	Type       DiffType
	OldMetric  *Metric
	NewMetric  *Metric
	Changes    []string
}

func compareMetrics(old, new map[MetricKey]Metric) []MetricDiff {
	var diffs []MetricDiff

	// Find added and modified metrics
	for key, newMetric := range new {
		if oldMetric, exists := old[key]; exists {
			// Check for modifications
			var changes []string
			if oldMetric.Help != newMetric.Help {
				changes = append(changes, "Help text changed")
			}
			if oldMetric.Type != newMetric.Type {
				changes = append(changes, fmt.Sprintf("Type changed from %s to %s", oldMetric.Type, newMetric.Type))
			}
			if oldMetric.StabilityLevel != newMetric.StabilityLevel {
				changes = append(changes, fmt.Sprintf("Stability level changed from %s to %s", oldMetric.StabilityLevel, newMetric.StabilityLevel))
			}
			if oldMetric.Namespace != newMetric.Namespace {
				changes = append(changes, fmt.Sprintf("Namespace changed from '%s' to '%s'", oldMetric.Namespace, newMetric.Namespace))
			}

			// Compare labels
			if !equalStringSlices(oldMetric.Labels, newMetric.Labels) {
				changes = append(changes, "Labels changed")
			}

			// Compare buckets
			if !equalFloat64Slices(oldMetric.Buckets, newMetric.Buckets) {
				changes = append(changes, "Buckets changed")
			}

			if len(changes) > 0 {
				diffs = append(diffs, MetricDiff{
					Key:       key,
					Type:      Modified,
					OldMetric: &oldMetric,
					NewMetric: &newMetric,
					Changes:   changes,
				})
			}
		} else {
			// Added metric
			diffs = append(diffs, MetricDiff{
				Key:       key,
				Type:      Added,
				NewMetric: &newMetric,
			})
		}
	}

	// Find removed metrics
	for key, oldMetric := range old {
		if _, exists := new[key]; !exists {
			diffs = append(diffs, MetricDiff{
				Key:       key,
				Type:      Removed,
				OldMetric: &oldMetric,
			})
		}
	}

	// Sort diffs by key for consistent output
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Key.String() < diffs[j].Key.String()
	})

	return diffs
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalFloat64Slices(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func printMarkdownTable(diffs []MetricDiff) {
	fmt.Println("# Kubernetes Metrics Changes: v1.33.0 → v1.34.0")
	fmt.Println()

	if len(diffs) == 0 {
		fmt.Println("No differences found between v1.33.0 and v1.34.0.")
		return
	}

	// Count changes by type
	var added, removed, modified int
	for _, diff := range diffs {
		switch diff.Type {
		case Added:
			added++
		case Removed:
			removed++
		case Modified:
			modified++
		}
	}

	fmt.Printf("## Summary\n")
	fmt.Printf("- **Added**: %d metrics\n", added)
	fmt.Printf("- **Removed**: %d metrics\n", removed)
	fmt.Printf("- **Modified**: %d metrics\n", modified)
	fmt.Printf("- **Total Changes**: %d\n\n", len(diffs))

	fmt.Println("## Detailed Changes")
	fmt.Println()
	fmt.Println("| Change Type | Namespace | Subsystem | Metric Name | Type | Stability Level | Description |")
	fmt.Println("|-------------|-----------|-----------|-------------|------|----------------|-------------|")

	for _, diff := range diffs {
		namespace := diff.Key.Namespace
		if namespace == "" {
			namespace = "-"
		}

		subsystem := diff.Key.Subsystem
		if subsystem == "" {
			subsystem = "-"
		}

		name := diff.Key.Name
		var metricType, stabilityLevel, description string

		switch diff.Type {
		case Added:
			metricType = diff.NewMetric.Type
			stabilityLevel = diff.NewMetric.StabilityLevel
			description = truncateString(diff.NewMetric.Help, 100)
		case Removed:
			metricType = diff.OldMetric.Type
			stabilityLevel = diff.OldMetric.StabilityLevel
			description = truncateString(diff.OldMetric.Help, 100)
		case Modified:
			metricType = diff.NewMetric.Type
			stabilityLevel = diff.NewMetric.StabilityLevel
			description = strings.Join(diff.Changes, "; ")
		}

		// Escape pipe characters in description
		description = strings.ReplaceAll(description, "|", "\\|")

		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s |\n",
			diff.Type, namespace, subsystem, name, metricType, stabilityLevel, description)
	}
}

func truncateString(s string, maxLen int) string {
	// Remove newlines and extra spaces
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.Join(strings.Fields(s), " ")

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <v1.33.0.yaml> <v1.34.0.yaml>\n", os.Args[0])
		os.Exit(1)
	}

	oldFile := os.Args[1]
	newFile := os.Args[2]

	oldMetrics, err := loadMetrics(oldFile)
	if err != nil {
		log.Fatalf("Error loading %s: %v", oldFile, err)
	}

	newMetrics, err := loadMetrics(newFile)
	if err != nil {
		log.Fatalf("Error loading %s: %v", newFile, err)
	}

	diffs := compareMetrics(oldMetrics, newMetrics)
	printMarkdownTable(diffs)
}