// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"time"
)

const (
	reportFileName = "scaler-report.json"
	eventsFileName = "scaler-events.json"
)

var metricsCSVHeader = []string{
	"timestamp", "cpu_millicores", "memory_mi", "memory_rss_mi",
	"memory_max_usage_mi", "memory_limit_mi",
	"cpu_throttled_periods", "cpu_total_periods", "cpu_throttled_time_ns", "pids",
}

func writeReports(dir string, meta RunMetadata, events []ScalingEvent) {
	if err := writeJSON(path.Join(dir, reportFileName), meta); err != nil {
		log.Printf("Failed to write report: %v\n", err)
	}
	if err := writeJSON(path.Join(dir, eventsFileName), events); err != nil {
		log.Printf("Failed to write events: %v\n", err)
	}
	log.Printf("Wrote reports to %s\n", dir)
}

func writeJSON(filePath string, v any) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", filePath, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("cannot encode %s: %w", filePath, err)
	}
	return nil
}

func writeMetricsCSV(dir string, metrics map[string][]ContainerStats) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		log.Printf("Failed to create metrics directory: %v\n", err)
		return
	}
	for name, stats := range metrics {
		filePath := path.Join(dir, name+".csv")
		if err := writeContainerCSV(filePath, stats); err != nil {
			log.Printf("Failed to write metrics CSV for %s: %v\n", name, err)
		}
	}
	log.Printf("Wrote metrics CSVs to %s\n", dir)
}

func writeContainerCSV(filePath string, stats []ContainerStats) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(metricsCSVHeader); err != nil {
		return err
	}
	for _, s := range stats {
		record := []string{
			s.Timestamp.UTC().Format(time.RFC3339),
			strconv.FormatUint(s.CPUMillicores, 10),
			strconv.FormatUint(s.MemoryMi, 10),
			strconv.FormatUint(s.MemoryRSSMi, 10),
			strconv.FormatUint(s.MemoryMaxUsageMi, 10),
			strconv.FormatUint(s.MemoryLimitMi, 10),
			strconv.FormatUint(s.CPUThrottledPeriods, 10),
			strconv.FormatUint(s.CPUTotalPeriods, 10),
			strconv.FormatUint(s.CPUThrottledTimeNs, 10),
			strconv.FormatUint(uint64(s.PID), 10),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
