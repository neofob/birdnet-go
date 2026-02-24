package observability

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// CPUUsageFunc is a function that returns the current total CPU usage percentage.
// This allows the collector to reuse the existing CPUCache from the API package
// instead of making a conflicting concurrent cpu.Percent call.
type CPUUsageFunc func() float64

// Collector periodically samples system metrics and records them into a MetricsStore.
type Collector struct {
	store    MetricsStore
	interval time.Duration
	cpuFunc  CPUUsageFunc

	// Internal state for disk I/O delta computation
	prevDiskIO   map[string]disk.IOCountersStat
	prevDiskTime time.Time

	// Track which metrics have had logged errors to avoid log spam
	loggedErrors map[string]bool
}

// NewCollector creates a Collector that samples system metrics at the given interval
// and stores them in the provided MetricsStore.
// The cpuFunc should return the current total CPU usage percentage (e.g. from CPUCache).
func NewCollector(store MetricsStore, interval time.Duration, cpuFunc CPUUsageFunc) *Collector {
	return &Collector{
		store:        store,
		interval:     interval,
		cpuFunc:      cpuFunc,
		prevDiskIO:   make(map[string]disk.IOCountersStat),
		loggedErrors: make(map[string]bool),
	}
}

// Start runs the collection loop until the context is cancelled.
// It should be called in a goroutine.
func (c *Collector) Start(ctx context.Context) {
	// Collect immediately on start, then on each tick
	c.collect()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// expectedMetricCount is the pre-allocation hint for the number of metrics collected per tick
// (cpu, memory, temperature, disk usage, disk IO read/write per device).
const expectedMetricCount = 8

// collect gathers all system metrics and records them as a single batch.
func (c *Collector) collect() {
	points := make(map[string]float64, expectedMetricCount)

	c.collectCPU(points)
	c.collectMemory(points)
	c.collectTemperature(points)
	c.collectDisk(points)

	if len(points) > 0 {
		c.store.RecordBatch(points)
	}
}

// collectCPU reads CPU usage from the injected function.
func (c *Collector) collectCPU(points map[string]float64) {
	if c.cpuFunc != nil {
		points["cpu.total"] = c.cpuFunc()
	}
}

// collectMemory reads memory usage via gopsutil.
func (c *Collector) collectMemory(points map[string]float64) {
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		c.logOnce("memory", "failed to collect memory metrics: %v", err)
		return
	}
	points["memory.used_percent"] = memInfo.UsedPercent
}

// collectTemperature reads CPU temperature from Linux thermal zones.
// Gracefully skipped on non-Linux or if no suitable sensor is found.
func (c *Collector) collectTemperature(points map[string]float64) {
	temp, ok := readCPUTemperature()
	if ok {
		points["cpu.temperature"] = temp
	}
}

// collectDisk reads disk usage and I/O statistics via gopsutil.
func (c *Collector) collectDisk(points map[string]float64) {
	c.collectDiskUsage(points)
	c.collectDiskIO(points)
}

// collectDiskUsage reads disk usage percentages for each partition.
func (c *Collector) collectDiskUsage(points map[string]float64) {
	partitions, err := disk.Partitions(false)
	if err != nil {
		c.logOnce("disk_partitions", "failed to list disk partitions: %v", err)
		return
	}

	for i := range partitions {
		p := &partitions[i]
		if skipCollectorFS(p.Fstype) {
			continue
		}
		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue // skip individual mount failures silently
		}
		key := fmt.Sprintf("disk.used_percent.%s", sanitizeMountpoint(p.Mountpoint))
		points[key] = usage.UsedPercent
	}
}

// collectDiskIO computes disk I/O rates (bytes/sec) as deltas between ticks.
func (c *Collector) collectDiskIO(points map[string]float64) {
	counters, err := disk.IOCounters()
	if err != nil {
		c.logOnce("disk_io", "failed to read disk I/O counters: %v", err)
		return
	}

	now := time.Now()
	if !c.prevDiskTime.IsZero() {
		elapsed := now.Sub(c.prevDiskTime).Seconds()
		if elapsed > 0 {
			for device := range counters {
				counter := counters[device]
				prev, ok := c.prevDiskIO[device]
				if !ok {
					continue
				}
				// Sanitize device name to remove any path prefixes (platform-dependent)
				name := filepath.Base(device)
				// Guard against counter resets (device swap, kernel rollover)
				if counter.ReadBytes >= prev.ReadBytes {
					readRate := float64(counter.ReadBytes-prev.ReadBytes) / elapsed
					points[fmt.Sprintf("disk.io.read.%s", name)] = readRate
				}
				if counter.WriteBytes >= prev.WriteBytes {
					writeRate := float64(counter.WriteBytes-prev.WriteBytes) / elapsed
					points[fmt.Sprintf("disk.io.write.%s", name)] = writeRate
				}
			}
		}
	}

	c.prevDiskIO = counters
	c.prevDiskTime = now
}

// logOnce logs a message for a metric category only on the first occurrence.
func (c *Collector) logOnce(category, format string, args ...any) {
	if c.loggedErrors[category] {
		return
	}
	c.loggedErrors[category] = true
	GetLogger().Debug(fmt.Sprintf(format, args...))
}

// sanitizeMountpoint converts a mountpoint path to a metric-safe name.
// e.g., "/" -> "root", "/home" -> "home", "/mnt/data" -> "mnt-data"
func sanitizeMountpoint(mount string) string {
	if mount == "/" {
		return "root"
	}
	// Remove leading slash, replace remaining slashes with dashes
	name := strings.TrimPrefix(mount, "/")
	return strings.ReplaceAll(name, "/", "-")
}

// skipCollectorFS returns true for virtual/pseudo filesystem types that should not be tracked.
func skipCollectorFS(fstype string) bool {
	skip := map[string]bool{
		"sysfs": true, "proc": true, "procfs": true, "devfs": true,
		"devtmpfs": true, "debugfs": true, "securityfs": true, "tmpfs": true,
		"ramfs": true, "overlay": true, "overlayfs": true, "fusectl": true,
		"devpts": true, "hugetlbfs": true, "mqueue": true, "cgroup": true,
		"cgroupfs": true, "pstore": true, "binfmt_misc": true, "bpf": true,
		"tracefs": true, "configfs": true, "autofs": true, "efivarfs": true,
	}
	return skip[fstype]
}

// --- CPU Temperature reading (Linux-specific) ---

// thermalBasePath is the base directory for thermal zones on Linux.
const collectorThermalBasePath = "/sys/class/thermal/"

// cpuThermalSensorTypes contains sensor type names that indicate CPU temperature.
var cpuThermalSensorTypes = map[string]bool{
	"cpu-thermal":     true,
	"x86_pkg_temp":    true,
	"soc_thermal":     true,
	"cpu_thermal":     true,
	"thermal-fan-est": true,
}

// readCPUTemperature scans Linux thermal zones for a CPU temperature sensor.
// Returns the temperature in Celsius and true if found, or 0 and false otherwise.
func readCPUTemperature() (float64, bool) {
	zones, err := filepath.Glob(filepath.Join(collectorThermalBasePath, "thermal_zone*"))
	if err != nil || len(zones) == 0 {
		return 0, false
	}

	for _, zone := range zones {
		temp, ok := readThermalZone(zone)
		if ok {
			return temp, true
		}
	}
	return 0, false
}

// readThermalZone reads a single thermal zone and returns its temperature
// if it matches a CPU thermal sensor type and has a valid reading.
func readThermalZone(zonePath string) (float64, bool) {
	// Read sensor type — paths are from filepath.Glob on /sys/class/thermal/, not user input.
	typeData, err := os.ReadFile(filepath.Join(zonePath, "type")) //nolint:gosec // system path from Glob
	if err != nil {
		return 0, false
	}
	sensorType := strings.ToLower(strings.TrimSpace(string(typeData)))
	if !cpuThermalSensorTypes[sensorType] {
		return 0, false
	}

	// Read temperature (in millidegrees Celsius)
	tempData, err := os.ReadFile(filepath.Join(zonePath, "temp")) //nolint:gosec // system path from Glob
	if err != nil {
		return 0, false
	}
	milliCelsius, err := strconv.Atoi(strings.TrimSpace(string(tempData)))
	if err != nil {
		return 0, false
	}

	const milliToUnit = 1000.0
	celsius := float64(milliCelsius) / milliToUnit

	// Validate range: 120°C upper bound captures overheating before thermal shutdown
	if celsius < 0 || celsius > 120 {
		return 0, false
	}
	return celsius, true
}
