package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Metric struct {
	Name              string              `yaml:"name" json:"name"`
	Subsystem         string              `yaml:"subsystem,omitempty" json:"subsystem,omitempty"`
	Namespace         string              `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Help              string              `yaml:"help,omitempty" json:"help,omitempty"`
	Type              string              `yaml:"type,omitempty" json:"type,omitempty"`
	DeprecatedVersion string              `yaml:"deprecatedVersion,omitempty" json:"deprecatedVersion,omitempty"`
	StabilityLevel    string              `yaml:"stabilityLevel,omitempty" json:"stabilityLevel,omitempty"`
	Labels            []string            `yaml:"labels,omitempty" json:"labels,omitempty"`
	Buckets           []float64           `yaml:"buckets,omitempty" json:"buckets,omitempty"`
	Objectives        map[float64]float64 `yaml:"objectives,omitempty" json:"objectives,omitempty"`
	AgeBuckets        uint32              `yaml:"ageBuckets,omitempty" json:"ageBuckets,omitempty"`
	BufCap            uint32              `yaml:"bufCap,omitempty" json:"bufCap,omitempty"`
	MaxAge            int64               `yaml:"maxAge,omitempty" json:"maxAge,omitempty"`
	ConstLabels       map[string]string   `yaml:"constLabels,omitempty" json:"constLabels,omitempty"`
}

func metricKey(m Metric) string {
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

func loadMetrics(filename string) (map[string]Metric, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var metrics []Metric
	err = yaml.Unmarshal(data, &metrics)
	if err != nil {
		return nil, err
	}

	metricMap := make(map[string]Metric)
	for _, metric := range metrics {
		key := metricKey(metric)
		metricMap[key] = metric
	}

	return metricMap, nil
}

type DiffType string

const (
	Added   DiffType = "Added"
	Removed DiffType = "Removed"
	Updated DiffType = "Updated"
)

type MetricDiff struct {
	Key       string
	Type      DiffType
	OldMetric *Metric
	NewMetric *Metric
	Changes   []string
}

func compareMetrics(old, new map[string]Metric) []MetricDiff {
	var diffs []MetricDiff

	// Find added and modified metrics
	for key, newMetric := range new {
		if oldMetric, exists := old[key]; exists {
			// Check for modifications
			var changes []string
			if oldMetric.Help != newMetric.Help {
				changes = append(changes, "Help text changed.")
			}
			if oldMetric.Type != newMetric.Type {
				changes = append(changes, fmt.Sprintf("Type changed from `%s` to `%s`.", oldMetric.Type, newMetric.Type))
			}
			if oldMetric.StabilityLevel != newMetric.StabilityLevel {
				changes = append(changes, fmt.Sprintf("Stability level changed from `%s` to `%s`.", oldMetric.StabilityLevel, newMetric.StabilityLevel))
			}
			if oldMetric.DeprecatedVersion != newMetric.DeprecatedVersion {
				if oldMetric.DeprecatedVersion == "" {
					changes = append(changes, fmt.Sprintf("Marked as deprecated in version `%s`.", newMetric.DeprecatedVersion))
				} else if newMetric.DeprecatedVersion == "" {
					changes = append(changes, "No longer marked as deprecated.")
				} else {
					changes = append(changes, fmt.Sprintf("Deprecated version changed from `%s` to `%s`.", oldMetric.DeprecatedVersion, newMetric.DeprecatedVersion))
				}
			}
			if oldMetric.AgeBuckets != newMetric.AgeBuckets {
				changes = append(changes, fmt.Sprintf("AgeBuckets changed from `%d` to `%d`.", oldMetric.AgeBuckets, newMetric.AgeBuckets))
			}
			if oldMetric.BufCap != newMetric.BufCap {
				changes = append(changes, fmt.Sprintf("BufCap changed from `%d` to `%d`.", oldMetric.BufCap, newMetric.BufCap))
			}
			if oldMetric.MaxAge != newMetric.MaxAge {
				changes = append(changes, fmt.Sprintf("MaxAge changed from `%d` to `%d`.", oldMetric.MaxAge, newMetric.MaxAge))
			}
			if reflect.DeepEqual(oldMetric.ConstLabels, newMetric.ConstLabels) == false {
				changes = append(changes, "ConstLabels changed.")
			}

			if !equalStringSlices(oldMetric.Labels, newMetric.Labels) {
				labelDiff := compareLabelSlices(oldMetric.Labels, newMetric.Labels)
				changes = append(changes, labelDiff)
			}

			if !equalFloat64Slices(oldMetric.Buckets, newMetric.Buckets) {
				changes = append(changes, "Buckets changed.")
			}

			if len(changes) > 0 {
				diffs = append(diffs, MetricDiff{
					Key:       key,
					Type:      Updated,
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
		return diffs[i].Key < diffs[j].Key
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

func compareLabelSlices(oldLabels, newLabels []string) string {
	oldSet := make(map[string]bool)
	newSet := make(map[string]bool)

	for _, label := range oldLabels {
		oldSet[label] = true
	}
	for _, label := range newLabels {
		newSet[label] = true
	}

	var added, removed []string

	// Find added labels
	for label := range newSet {
		if !oldSet[label] {
			added = append(added, fmt.Sprintf("`%s`", label))
		}
	}

	// Find removed labels
	for label := range oldSet {
		if !newSet[label] {
			removed = append(removed, fmt.Sprintf("`%s`", label))
		}
	}

	sort.Strings(added)
	sort.Strings(removed)

	var changes []string
	if len(added) > 0 {
		changes = append(changes, fmt.Sprintf("Added labels: [%s].", strings.Join(added, ", ")))
	}
	if len(removed) > 0 {
		changes = append(changes, fmt.Sprintf("Removed labels: [%s].", strings.Join(removed, ", ")))
	}

	if len(changes) == 0 {
		// Labels exist but order changed
		return fmt.Sprintf("Labels reordered: [%s] → [%s]", strings.Join(oldLabels, ", "), strings.Join(newLabels, ", "))
	}

	return strings.Join(changes, " <br> ")
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

func printMarkdownTable(diffs []MetricDiff, oldVersion, newVersion string) {
	fmt.Printf("# Kubernetes Metrics Changes: %s → %s\n", oldVersion, newVersion)
	fmt.Println()

	if len(diffs) == 0 {
		fmt.Printf("No differences found between %s and %s.\n", oldVersion, newVersion)
		return
	}

	// Count changes by type
	var added, removed, updated int
	for _, diff := range diffs {
		switch diff.Type {
		case Added:
			added++
		case Removed:
			removed++
		case Updated:
			updated++
		}
	}

	fmt.Printf("## Summary\n")
	fmt.Printf("- **Added**: %d metrics\n", added)
	fmt.Printf("- **Removed**: %d metrics\n", removed)
	fmt.Printf("- **Updated**: %d metrics\n", updated)
	fmt.Printf("- **Total Changes**: %d metrics\n\n", len(diffs))

	fmt.Println("## Changed Metrics")
	fmt.Println()
	fmt.Println("| Metric Name | Type | Change Type | Stability Level | Description |")
	fmt.Println("|-------------|------|-------------|----------------|-------------|")

	for _, diff := range diffs {
		name := diff.Key

		var metricType, stabilityLevel, description string

		switch diff.Type {
		case Added:
			metricType = diff.NewMetric.Type
			stabilityLevel = diff.NewMetric.StabilityLevel
			// description = truncateString(diff.NewMetric.Help, 100)
		case Removed:
			metricType = diff.OldMetric.Type
			stabilityLevel = diff.OldMetric.StabilityLevel
			// description = truncateString(diff.OldMetric.Help, 100)
		case Updated:
			metricType = diff.NewMetric.Type
			stabilityLevel = diff.NewMetric.StabilityLevel
			description = strings.Join(diff.Changes, " <br> ")
		}

		// Escape pipe characters in description
		description = strings.ReplaceAll(description, "|", "\\|")

		fmt.Printf("| [%s](#%s) | %s | %s | `%s` | %s |\n",
			name, name, metricType, diff.Type, stabilityLevel, description)
	}

	fmt.Println("## Detailed Changes")
	fmt.Println()

	for _, diff := range diffs {
		fmt.Printf("### %s\n", diff.Key)

		var old, new []byte
		var err error

		if diff.OldMetric != nil {
			old, err = yaml.Marshal([]any{diff.OldMetric})
			if err != nil {
				log.Fatalf("Error marshaling old metric: %v", err)
			}
		}
		if diff.NewMetric != nil {
			new, err = yaml.Marshal([]any{diff.NewMetric})
			if err != nil {
				log.Fatalf("Error marshaling new metric: %v", err)
			}
		}

		ud := unifiedDiffWithoutHeader(string(old), string(new))

		fmt.Println("```diff")
		fmt.Print(ud)
		fmt.Println("```")
		fmt.Println()
	}
}

func unifiedDiffWithoutHeader(old, new string) string {
	oldFile, err := os.CreateTemp("", "")
	if err != nil {
		log.Fatalf("Error creating temp file: %v", err)
	}
	defer os.Remove(oldFile.Name())
	defer oldFile.Close()

	newFile, err := os.CreateTemp("", "")
	if err != nil {
		log.Fatalf("Error creating temp file: %v", err)
	}
	defer os.Remove(newFile.Name())
	defer newFile.Close()

	if _, err := oldFile.WriteString(old); err != nil {
		log.Fatalf("Error writing to temp file: %v", err)
	}
	if _, err := newFile.WriteString(new); err != nil {
		log.Fatalf("Error writing to temp file: %v", err)
	}

	output, err := exec.Command("diff", "-U999999", oldFile.Name(), newFile.Name()).CombinedOutput()
	if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != 1 {
		log.Fatalf("Error running diff command: %s %v", string(output), err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) <= 3 {
		return ""
	}
	return strings.Join(lines[3:], "\n")
}

func versionFromPath(path string) string {
	base := filepath.Base(path)

	return strings.TrimSuffix(base, filepath.Ext(base))
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <old.yaml> <new.yaml>\n", os.Args[0])
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

	oldVersion := versionFromPath(oldFile)
	newVersion := versionFromPath(newFile)

	diffs := compareMetrics(oldMetrics, newMetrics)
	printMarkdownTable(diffs, oldVersion, newVersion)
}
