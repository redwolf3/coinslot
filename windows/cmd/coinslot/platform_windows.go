package main

import (
	"github.com/redwolf3/coinslot/windows/collector"
	"github.com/redwolf3/coinslot/windows/internal/win32"
)

func newPlatform() (collector.Probe, uint32, error) {
	p, err := win32.NewProbe()
	if err != nil {
		return nil, 0, err
	}
	return p, p.SessionID, nil
}

func singleInstance(name string) (bool, error) { return win32.SingleInstance(name) }
