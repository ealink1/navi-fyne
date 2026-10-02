package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

var footprintPattern = regexp.MustCompile(`phys_footprint:\s+(\d+) B`)

func physicalFootprint(ctx context.Context) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/bin/footprint", "-p", strconv.Itoa(os.Getpid()), "--noCategories", "-f", "bytes").Output()
	if err != nil {
		return 0, fmt.Errorf("native footprint measurement: %w", err)
	}
	match := footprintPattern.FindSubmatch(output)
	if len(match) != 2 {
		return 0, fmt.Errorf("native footprint measurement returned an unknown format")
	}
	return strconv.ParseUint(string(match[1]), 10, 64)
}
