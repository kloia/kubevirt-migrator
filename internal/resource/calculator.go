package resource

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// ResourceCalculator handles dynamic resource calculations
type ResourceCalculator struct {
	logger *zap.Logger
}

// Resources holds CPU and memory resource specifications
type Resources struct {
	CPULimit      string
	CPURequest    string
	MemoryLimit   string
	MemoryRequest string
}

// NewResourceCalculator creates a new resource calculator
func NewResourceCalculator(logger *zap.Logger) *ResourceCalculator {
	return &ResourceCalculator{
		logger: logger,
	}
}

// ParseSize parses a k8s size string (e.g., "10Gi") into bytes
func (r *ResourceCalculator) ParseSize(size string) (int64, error) {
	size = strings.TrimSpace(size)
	if len(size) == 0 {
		return 0, fmt.Errorf("empty size string")
	}

	// Get the numeric part and unit
	var numStr string
	var unit string
	for i, c := range size {
		if !('0' <= c && c <= '9') {
			numStr = size[:i]
			unit = size[i:]
			break
		}
		if i == len(size)-1 {
			numStr = size
			unit = ""
		}
	}

	// Parse the numeric part
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size format: %s", size)
	}

	// Convert to bytes based on unit
	unit = strings.ToLower(unit)
	switch unit {
	case "k", "ki":
		return num * 1024, nil
	case "m", "mi":
		return num * 1024 * 1024, nil
	case "g", "gi":
		return num * 1024 * 1024 * 1024, nil
	case "t", "ti":
		return num * 1024 * 1024 * 1024 * 1024, nil
	case "":
		return num, nil
	default:
		return 0, fmt.Errorf("unknown unit in size: %s", unit)
	}
}

// CalculateResourcesFromUsage calculates appropriate CPU and memory resources based on actual disk usage
func (r *ResourceCalculator) CalculateResourcesFromUsage(usedBytes int64) (Resources, error) {
	// Calculate used size in GB
	usedGB := float64(usedBytes) / (1024 * 1024 * 1024)

	// Set minimum 1GB usage for very small VMs
	if usedGB < 1 {
		usedGB = 1
	}

	// CPU (cores) - 1 core for each 5GB of used data with min 1, max 4
	cpuCores := math.Ceil(usedGB / 5.0)
	if cpuCores < 1 {
		cpuCores = 1 // Minimum 1 core
	} else if cpuCores > 4 {
		cpuCores = 4 // Maximum 4 cores
	}

	// Memory (GB) - 0.3x the used data size with min 2GB, max 8GB
	// Reduced from 1.5x to 0.3x as cp/rclone operations are more CPU intensive than memory intensive
	memoryGB := math.Ceil(usedGB * 0.3)
	if memoryGB < 2 {
		memoryGB = 2 // Minimum 2GB RAM
	} else if memoryGB > 8 {
		memoryGB = 8 // Maximum 8GB RAM (reduced from 16GB)
	}

	// Set request at 70% of limit
	resources := Resources{
		CPULimit:      fmt.Sprintf("%.1f", cpuCores),
		CPURequest:    fmt.Sprintf("%.1f", cpuCores*0.7),
		MemoryLimit:   fmt.Sprintf("%.0fGi", memoryGB),
		MemoryRequest: fmt.Sprintf("%.0fGi", math.Ceil(memoryGB*0.7)),
	}

	r.logger.Info("Calculated resources based on actual disk usage",
		zap.Float64("usedGB", usedGB),
		zap.String("cpuLimit", resources.CPULimit),
		zap.String("cpuRequest", resources.CPURequest),
		zap.String("memoryLimit", resources.MemoryLimit),
		zap.String("memoryRequest", resources.MemoryRequest))

	return resources, nil
}

// FallbackToPVCSize calculates resources based on PVC size when actual usage is unavailable
func (r *ResourceCalculator) FallbackToPVCSize(pvcSize string) (Resources, error) {
	// Convert PVC size to bytes
	sizeBytes, err := r.ParseSize(pvcSize)
	if err != nil {
		return Resources{}, err
	}

	// Assume 25% of PVC is actually used
	estimatedUsage := int64(float64(sizeBytes) * 0.25)

	// Calculate based on estimated usage
	resources, err := r.CalculateResourcesFromUsage(estimatedUsage)
	if err != nil {
		return Resources{}, err
	}

	r.logger.Info("Calculated resources based on PVC size (fallback method)",
		zap.String("pvcSize", pvcSize),
		zap.Int64("estimatedUsage", estimatedUsage))

	return resources, nil
}

// GetDefaultResources returns sensible default resource values
func (r *ResourceCalculator) GetDefaultResources() Resources {
	return Resources{
		CPULimit:      "1",
		CPURequest:    "1",
		MemoryLimit:   "2Gi",
		MemoryRequest: "2Gi",
	}
}
