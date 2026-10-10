//go:build !windows && !linux

package main

import "errors"

func NewBackend(forceVT, forceLegacy bool) Backend { return &stubBackend{} }

type stubBackend struct{}

func (*stubBackend) Init() error                         { return errors.New("Dot supports Windows (and Linux for testing)") }
func (*stubBackend) Close()                              {}
func (*stubBackend) Size() (int, int)                    { return 80, 25 }
func (*stubBackend) Flush(*Screen, int, int, bool)       {}
func (*stubBackend) Events() <-chan Event                { return nil }
func (*stubBackend) GetClip() (string, bool)             { return "", false }
func (*stubBackend) SetClip(string)                      {}
func (*stubBackend) Resize(int, int) bool                { return false }
