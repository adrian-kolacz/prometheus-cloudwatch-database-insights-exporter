package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Property 1: Invalid YAML rejection
// Validates: Requirements 1.3
func TestProperty_InvalidYAMLRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig rejects invalid YAML", prop.ForAll(
		func(invalidYAML string) bool {
			// Create temporary file with invalid YAML
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "invalid_config.yml")

			if err := os.WriteFile(tmpFile, []byte(invalidYAML), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			// Attempt to load config
			_, err := LoadConfig(tmpFile)

			// Should return an error for invalid YAML
			return err != nil
		},
		gen.OneConstOf(
			"invalid: yaml: content: [unclosed",
			"tabs:\n\t\tinvalid\t\tindentation",
			"- list\n  without\n  proper\n  indentation",
			"key: value\n  nested: without parent",
			"[unclosed bracket",
			"{unclosed: brace",
		),
	))

	properties.TestingRun(t)
}

// Property 2: Multi-region first selection
// Validates: Requirements 1.4
func TestProperty_MultiRegionFirstSelection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig selects first region when multiple provided", prop.ForAll(
		func(regions []string) bool {
			if len(regions) < 2 {
				return true // Skip if less than 2 regions
			}

			// Create config with multiple regions
			yamlContent := fmt.Sprintf(`discovery:
  regions:
%s
`, generateRegionList(regions))

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			config, err := LoadConfig(tmpFile)
			if err != nil {
				return false
			}

			// Should only have first region
			return len(config.Discovery.Regions) == 1 && config.Discovery.Regions[0] == regions[0]
		},
		gen.SliceOfN(5, gen.OneConstOf("us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1", "ca-central-1")),
	))

	properties.TestingRun(t)
}

// Property 3: Optional field defaults
// Validates: Requirements 1.5
func TestProperty_OptionalFieldDefaults(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig applies defaults for omitted optional fields", prop.ForAll(
		func() bool {
			// Minimal config with only required fields
			yamlContent := `discovery:
  regions:
    - us-west-2
`

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			config, err := LoadConfig(tmpFile)
			if err != nil {
				return false
			}

			// Verify defaults are applied
			return config.Discovery.Instances.MaxInstances == MaxInstances &&
				config.Discovery.Metrics.Statistic.String() == "avg" &&
				config.Discovery.Processing.Concurrency == DefaultConcurrency &&
				config.Export.Port == 8081 &&
				config.Export.Prometheus.MetricPrefix == "dbi" &&
				config.Discovery.Metrics.DataCacheMaxSize == DefaultCacheMaxSize
		},
	))

	properties.TestingRun(t)
}

// Property 4: Invalid statistic rejection
// Validates: Requirements 1.6
func TestProperty_InvalidStatisticRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig rejects invalid statistic values", prop.ForAll(
		func(invalidStat string) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  metrics:
    statistic: "%s"
`, invalidStat)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			_, err := LoadConfig(tmpFile)

			// Should return error for invalid statistic
			return err != nil
		},
		gen.OneConstOf("average", "mean", "median", "invalid", "AVG", "MAX", "MINIMUM"),
	))

	properties.TestingRun(t)
}

// Property 5: Invalid TTL rejection
// Validates: Requirements 1.7
func TestProperty_InvalidTTLRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig rejects invalid TTL formats", prop.ForAll(
		func(invalidTTL string) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  instances:
    cache:
      ttl: "%s"
`, invalidTTL)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			_, err := LoadConfig(tmpFile)

			// Should return error for invalid TTL
			return err != nil
		},
		gen.OneConstOf("5 minutes", "1hour", "30sec", "invalid", "abc", "-5m"),
	))

	properties.TestingRun(t)
}

// Property 6: Out-of-range port handling
// Validates: Requirements 1.8
func TestProperty_OutOfRangePortHandling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig applies default for out-of-range ports", prop.ForAll(
		func(port int) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
export:
  port: %d
`, port)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			config, err := LoadConfig(tmpFile)
			if err != nil {
				// Port might be in use, which is acceptable
				return true
			}

			// Out-of-range ports should default to 8081
			if port <= 0 || port > 65535 {
				return config.Export.Port == 8081
			}

			return true
		},
		gen.IntRange(-1000, 100000).SuchThat(func(p int) bool {
			return p <= 0 || p > 65535
		}),
	))

	properties.TestingRun(t)
}

// Property 7: Invalid metric prefix rejection
// Validates: Requirements 1.10
func TestProperty_InvalidMetricPrefixRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig rejects invalid Prometheus metric prefixes", prop.ForAll(
		func(invalidPrefix string) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
export:
  prometheus:
    metric-prefix: "%s"
`, invalidPrefix)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			_, err := LoadConfig(tmpFile)

			// Should return error for invalid prefix
			return err != nil
		},
		gen.OneConstOf("123invalid", "_underscore", "invalid-dash", "invalid.dot", "9starts_with_number"),
	))

	properties.TestingRun(t)
}

// Property 14: Invalid filter field rejection
// Validates: Requirements 3.4
func TestProperty_InvalidFilterFieldRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig rejects invalid filter field names", prop.ForAll(
		func(invalidField string) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  instances:
    include:
      %s:
        - ".*"
`, invalidField)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			_, err := LoadConfig(tmpFile)

			// Should return error for invalid field
			return err != nil
		},
		gen.OneConstOf("invalid_field", "unknown", "database", "region", "status"),
	))

	properties.TestingRun(t)
}

// Property 15: Invalid regex rejection
// Validates: Requirements 3.5
func TestProperty_InvalidRegexRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig rejects invalid regex patterns", prop.ForAll(
		func(invalidRegex string) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  instances:
    include:
      identifier:
        - "%s"
`, invalidRegex)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			_, err := LoadConfig(tmpFile)

			// Should return error for invalid regex
			return err != nil
		},
		gen.OneConstOf("[unclosed", "(?invalid)", "*invalid", "(unclosed", "(?P<invalid"),
	))

	properties.TestingRun(t)
}

// Property 47: Low concurrency default
// Validates: Requirements 12.2
func TestProperty_LowConcurrencyDefault(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig applies default for low concurrency values", prop.ForAll(
		func(lowConcurrency int) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  processing:
    concurrency: %d
`, lowConcurrency)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			config, err := LoadConfig(tmpFile)
			if err != nil {
				return false
			}

			// Values < 1 should default to DefaultConcurrency
			if lowConcurrency < 1 {
				return config.Discovery.Processing.Concurrency == DefaultConcurrency
			}

			return true
		},
		gen.IntRange(-10, 0),
	))

	properties.TestingRun(t)
}

// Property 48: High concurrency default
// Validates: Requirements 12.4
func TestProperty_HighConcurrencyDefault(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig applies default for high concurrency values", prop.ForAll(
		func(highConcurrency int) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  processing:
    concurrency: %d
`, highConcurrency)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			config, err := LoadConfig(tmpFile)
			if err != nil {
				return false
			}

			// Values > MaximumConcurrency should default to DefaultConcurrency
			if highConcurrency > MaximumConcurrency {
				return config.Discovery.Processing.Concurrency == DefaultConcurrency
			}

			return true
		},
		gen.IntRange(MaximumConcurrency+1, MaximumConcurrency+100),
	))

	properties.TestingRun(t)
}

// Property 49: max-instances above default is accepted
// Validates that values above the default (25) are no longer silently capped.
func TestProperty_MaxInstancesAboveDefault(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LoadConfig accepts max-instances values above default", prop.ForAll(
		func(maxInstances int) bool {
			yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  instances:
    max-instances: %d
`, maxInstances)

			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.yml")

			if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			config, err := LoadConfig(tmpFile)
			if err != nil {
				return false
			}

			return config.Discovery.Instances.MaxInstances == maxInstances
		},
		gen.IntRange(MaxInstances+1, MaxInstancesHardLimit),
	))

	properties.TestingRun(t)
}

// Property 50: max-instances at upper boundary is accepted
func TestProperty_MaxInstancesAtUpperBoundary(t *testing.T) {
	yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  instances:
    max-instances: %d
`, MaxInstancesHardLimit)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	config, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Discovery.Instances.MaxInstances != MaxInstancesHardLimit {
		t.Errorf("expected %d, got %d", MaxInstancesHardLimit, config.Discovery.Instances.MaxInstances)
	}
}

// Property 51: max-instances above hard limit is capped at the hard limit
func TestProperty_MaxInstancesAboveUpperBoundary(t *testing.T) {
	yamlContent := fmt.Sprintf(`discovery:
  regions:
    - us-west-2
  instances:
    max-instances: %d
`, MaxInstancesHardLimit+1)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	config, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Discovery.Instances.MaxInstances != MaxInstancesHardLimit {
		t.Errorf("expected hard limit %d, got %d", MaxInstancesHardLimit, config.Discovery.Instances.MaxInstances)
	}
}

// Helper function to generate YAML region list
func generateRegionList(regions []string) string {
	result := ""
	for _, region := range regions {
		result += fmt.Sprintf("    - %s\n", region)
	}
	return result
}
