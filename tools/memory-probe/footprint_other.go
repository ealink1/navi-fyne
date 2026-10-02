//go:build !darwin

package main

import "context"

func physicalFootprint(context.Context) (uint64, error) { return 0, nil }
