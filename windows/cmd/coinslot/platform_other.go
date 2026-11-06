//go:build !windows

package main

import (
	"errors"

	"github.com/redwolf3/coinslot/windows/collector"
)

var errWindowsOnly = errors.New("the collector only runs on Windows")

func newPlatform() (collector.Probe, uint32, error) { return nil, 0, errWindowsOnly }

func singleInstance(string) (bool, error) { return false, errWindowsOnly }
